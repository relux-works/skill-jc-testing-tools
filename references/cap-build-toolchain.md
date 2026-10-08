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
   helper refactoring is not yet validated.
2. If the probe demonstrably succeeds and remains target-compatible, pin the
   exact converter distribution, record its SHA-256 digest, make the build
   reproducible, and rebuild every consumer CAP in the coordinated release.
3. If that route fails or emits an incompatible CAP, shorten the encoded
   handler distances by moving the protected region into a small helper. Keep
   the original busy-state, wipe, and release guarantees on every exit, then
   rerun the original positive controls, named negative tests, and narrowing
   mutants that prove those guarantees were not weakened.

Do not binary-patch `tools.jar`, waive CAP generation, remove `finally`, or
shuffle methods arbitrarily to make the offset disappear. A passing CAP probe
still requires the normal installation and physical-hardware qualification;
neither is implied by this diagnostic.
