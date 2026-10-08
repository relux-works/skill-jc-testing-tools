# CAP build toolchain: ant-javacard

Building a real CAP file (not just running under jCardSim) uses [ant-javacard](https://github.com/martinpaljak/ant-javacard) against an Oracle JavaCard SDK kit, driven by an Ant `<cap>` target.

## Minimal `cap-build.xml` shape

```xml
<project name="myapplet-cap-build" default="dist">
    <property name="toolchain.dir" location=".../cap-toolchain"/>
    <property name="ant.javacard.jar" location="${toolchain.dir}/ant-javacard.jar"/>
    <property name="jckit.name" value="jc320v25.1_kit"/> <!-- override per target JC version -->
    <property name="jckit.dir" location="${toolchain.dir}/oracle_javacard_sdks/${jckit.name}"/>

    <taskdef name="javacard" classname="pro.javacard.ant.JavaCard" classpath="${ant.javacard.jar}"/>

    <target name="dist">
        <javacard>
            <cap jckit="${jckit.dir}"
                 sources="src/main/java"
                 package="com.example.myapplet"
                 aid="F0000000AA"
                 version="0.1"
                 ints="true"
                 output="artifacts/myapplet.cap"
                 jca="artifacts/myapplet.jca">
                <applet class="com.example.myapplet.MyJCApplet" aid="F0000000AA01"/>
            </cap>
        </javacard>
    </target>
</project>
```

## Bootstrap the toolchain (once, reusable across projects)

```bash
mkdir -p .temp/cap-toolchain
curl -fsSL https://github.com/martinpaljak/ant-javacard/releases/latest/download/ant-javacard.jar \
  -o .temp/cap-toolchain/ant-javacard.jar
git clone --depth 1 https://github.com/martinpaljak/oracle_javacard_sdks.git \
  .temp/cap-toolchain/oracle_javacard_sdks
```

This produces many kit directories (`jc211_kit` through `jc320v25.1_kit`) -- you don't need to re-download per project; point `jckit.dir` at an existing bootstrapped toolchain from a sibling project if one exists.

## Gotcha 1: the CAP's target JC version must match the card's actual runtime, not just "the newest available"

Building against the newest kit (e.g. `jc320v25.1_kit`, JavaCard 3.0.5) will succeed and pass verification, but **`LOAD` can still fail on the physical card** with `0x6438` (Imported package not available) if the card's real runtime is an older JC version (e.g. 3.0.4) that doesn't support the `3.0.5` API level your CAP declares. This is a real-hardware-only failure -- nothing about the build or the simulator will warn you.

If you hit `0x6438` at `LOAD`, rebuild against a lower kit (`jc304_kit`, etc.) and retry. There's no way to know the card's actual ceiling in advance without vendor docs (which may not exist for a given test card) -- discovering it by trying progressively lower kits after a `LOAD` failure is the practical method.

## Gotcha 2: older JC kits need an older JDK to run `ant-javacard`'s own compile step

```
BUILD FAILED
.../cap-build.xml:NN: Can't use JDK 17 with JavaCard kit 3.0.4 (use JDK 11)
```

`ant-javacard` enforces this itself. Install the specific JDK version it demands (e.g. `brew install openjdk@11` on macOS) and invoke `ant` with an explicit `JAVA_HOME`/`PATH` override for that one build, rather than changing your system-wide JDK:

```bash
JDK11=/opt/homebrew/opt/openjdk@11/libexec/openjdk.jdk/Contents/Home
JAVA_HOME=$JDK11 PATH="$JDK11/bin:$PATH" \
  ant -f cap-build.xml -Djckit.name=jc304_kit dist
```

## Gotcha 3: `ints="true"`

Required if your applet code has any `int`-typed local variable at all (not just literal `int` constants) -- otherwise the converter rejects general `int` arithmetic outright, separately from the array-indexing rule in [codegen-jc-classic-compatibility.md](codegen-jc-classic-compatibility.md). Confirm the physical card actually supports the optional "int" capability by checking that `LOAD`+`INSTALL` succeed for real (a successful *build* with `ints="true"` does not by itself prove the card supports it at runtime -- it only proves the converter accepted the bytecode).

## Gotcha 4: Oracle 3.0.5u4 can overflow exception-handler reference locations

`javac` success, or even the converter writing EXP and JCA files, does **not**
mean that CAP generation succeeded and says nothing about physical-card
qualification. One recorded build used Oracle Java Card 3.0.5u4,
ant-javacard 26.02.22, and OpenJDK 11. Compilation and EXP/JCA generation
succeeded, but CAP generation failed inside the closed Oracle `tools.jar`:

```text
ReferenceLocationComponent.addException
  -> addTwoByteOffset(464)
  -> IllegalArgumentException
```

The method had accumulated a distance of 458 across many catch-all handlers
generated for exception-safe `finally` regions, then passed `458 + 6 = 464`
directly to a single-byte encoder. It did not emit the required continuation
entry. This is an observed defect in that pinned converter, not an
`ant-javacard` defect, a Java Card language restriction, proof that the source
guard is impossible, or evidence about physical-card behavior.

Oracle's **Java Card VM Specification 3.1**, section 6.12.1, defines a
two-byte reference-location distance greater than or equal to 255 as one or
more `255` continuation entries followed by the remainder, including entries
with a nonzero `catch_type_index` ([official PDF, pages 129--130](https://docs.oracle.com/en/java/javacard/3.1/jc-vm-spec/F12650_05.pdf)).
Exact multiples therefore end with a remainder of `0` (for example, distance
`255` is encoded as `255, 0`), rather than omitting the final remainder entry.
This is a 3.1 specification citation; it is not a claim that an older 3.0.5
specification PDF was inspected.

Diagnose and recover in this order:

1. Preserve the original source, pinned toolchain identity, and full compiler
   and converter diagnostics. In an isolated probe, use an official newer
   converter with explicit `-target 3.0.5` and the same 3.0.5 API export files.
   Oracle's **Java Card Development Kit User Guide 3.1**, table 5-1, says this
   target produces compact CAP 2.2, while the converter's default 3.1 target
   changes the CAP format ([official PDF](https://docs.oracle.com/en/java/javacard/3.1/guide/java-card-development-kit-user-guide.pdf)).
   That documentation establishes target support only: do not claim that the
   3.1 converter fixes this encoder defect until the probe produces a real CAP
   and compatibility evidence.
   The bounded recorded probe of the installed `jc310r20210706` Oracle 3.1
   converter, with explicit `-target 3.0.5` and the original 3.0.5 API exports,
   also failed CAP generation: `javac` exited 0, EXP/JCA generation succeeded,
   then `java.lang.IllegalArgumentException` caused `REAL_EXIT1`. Its
   `tools.jar` SHA-256 is
   `179b4eda4cea2d058b2c239f0d5e2b98940faced7f6b4e8287af7718df3184dd`.
   This is one recorded 3.1 build, not evidence about every newer converter;
   it did not recover packaging.
2. Measure the aggregate exception-handler layout before selecting a source
   structure. `ReferenceLocationComponent.f_byte2Offset` is package-wide
   static state, initialized once for the component rather than reset per
   method or class. In one retained failed route-B witness, 20 short execution
   helpers still put 53 catch-all entries before the first typed native-factory
   handler; that handler reached offset `431`, which exceeded the converter's
   `255` single-entry bound. The `431`/`53` observation describes that exact
   layout, not a universal handler-count limit. A short method by itself is
   therefore not a guaranteed recovery.
3. If the probe demonstrably succeeds and remains target-compatible, pin the
   exact converter distribution, record its SHA-256 digest, make the build
   reproducible, and rebuild every consumer CAP in the coordinated release.
4. If the ownership contract authorizes caller-owned cleanup, a structural
   alternative is explicit library `acquire` and wiping `release`, with each
   scratch primitive first verifying the current owner/operation marker and
   refusing busy (`0x698E`) before writing any live window, operand, or output.
   Protect each actual APDU-command and SIO entrypoint (including Facade/Auth
   consumers) with exactly one `try`/`finally` that performs the release.
   Cleanup moves to that boundary; it is not removed. Nested acquire must
   refuse any held marker, including the same operation. Do not retain a
   caller APDU array in global state; pass the current window to the wiping
   release when necessary.
5. Prove the selected structure against its real aggregate handler counts and
   reference gaps with the pinned Java Card 3.0.5u4 / ant-javacard 26.02.22 /
   OpenJDK 11 CAP gate. Preserve success, busy-refusal, native-exception,
   interruption/reset/retry, and genuine nested-SIO cleanup behavior; verify
   unchanged outer spans, persistent-output refusal, and complete transient
   GCM-output wiping. Keep the existing positive and negative controls and add
   narrowing `missing-primitive-owner-check` and
   `missing-entrypoint-release` mutants. If the aggregate bound still fails,
   stop and report the actual counts and offsets instead of weakening the
   ownership or cleanup contract.

One caller-owned diagnostic layout has produced a CAP on that unchanged pinned
toolchain: Oracle verification passed for 10 components, four artifacts, and
two class files at major version 50. This was one conversion using the existing
`1.1.3` artifact name. It is evidence that this particular packaging layout
recovered, not a universal cure, final `1.2.0` release identity, a
reproducibility result, completed behavioral or mutant acceptance, or physical
qualification.

Do not binary-patch `tools.jar`, waive CAP generation, add fake typed catches,
shuffle class or method order, introduce union flags, remove cleanup, or weaken
ownership to make the offset disappear. A passing CAP probe still requires the
normal source acceptance, reproducibility, installation, and physical-hardware
qualification; none is implied by this diagnostic.
