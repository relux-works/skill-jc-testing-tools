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
