# Java Card Classic compatibility rules for generated/hand-written applet code

jCardSim runs your applet code on a real JVM. That means it tolerates a lot of code that will **not** convert on a real Java Card Classic CAP converter. A CAP that builds successfully against `javac` and passes jCardSim tests can still fail the real `ant-javacard`/JC-converter step outright, or worse, convert successfully but do the wrong thing (or nothing) once actually running on real hardware. These rules were found the hard way, fixing 3 real bugs in `javacard-rpc`'s generated Java skeleton (`gen_java.go`) that had never been caught because that project's own test suite/examples never exercised a real physical CAP build, only jCardSim.

## Rule 1: no `java.lang.String`-based exceptions or string manipulation

`String` construction/concatenation, `StringBuilder`, `Integer.toHexString`, `String.toUpperCase()`, and exception constructors that take a `String` message (`new RuntimeException("...")`, `new IllegalArgumentException("...")`) are **not** part of the safely convertible Java Card Classic subset. Symptoms at convert time:

```
error: line N: com.example.Foo: unsupported String type constant.
error: line N: com.example.Foo: unsupported parameter type String of invoked method <init>(java.lang.String) of class java.lang.RuntimeException.
```

Fix: exceptions used inside applet-executed code should carry only numeric fields (e.g. a status word), not a message string. `throw new MyException(statusWord)` with a no-arg `super()` call, and a `getStatusWord()` accessor -- callers read the numeric field, not `getMessage()`.

## Rule 2: array indices and `new byte[...]` sizes must be `short` or `byte`, even with `ints="true"`

`ant-javacard`'s `<cap ints="true">` attribute enables general `int` arithmetic (needed just to have `int`-typed offset/length variables at all without erroring), but it does **not** relax the JCVM's separate requirement that the actual operand fed to an array load/store (`buf[off]`) or array-creation (`new byte[len]`) opcode be `short`- or `byte`-typed. An `int`-typed variable used directly as an index/size fails at convert time:

```
error: line N: com.example.Foo: unsupported int type array index, must cast array index to type short or byte.
```

Fix: keep the `int` parameter types if you want (simpler diffs, general arithmetic still works), but cast explicitly at every actual array-index and dynamic-`new byte[...]`-size site: `buf[(short) off]`, `new byte[(short) len]`.

## Rule 3: `System.arraycopy` does not exist on the real Java Card Classic `java.lang.System` stub

This one is sneaky because it compiles fine (`javac` and jCardSim both run on a real JVM with a real `System.arraycopy`) and only fails at the CAP-convert step, with an error that names the *method*, not an obviously-related keyword:

```
error: line N: com.example.Foo: class java.lang.System not found in export file lang.exp.
error: line N: com.example.Foo: method arraycopy(java.lang.Object, int, java.lang.Object, int, int) of class java.lang.System not found in export file lang.exp or the method signature has changed.
```

The "obvious" fix, `javacard.framework.Util.arrayCopyNonAtomic(...)`, **does** convert -- but only reach for it if your code is allowed to import `javacard.framework`. If the code needs to stay platform-agnostic (compilable as plain JVM code with zero JC-specific imports -- e.g. a generated skeleton meant to be testable and reusable outside a JC-specific build), that import is itself a regression against that goal.

The actual fix used in `javacard-rpc`'s generated skeleton: a manual short-indexed copy loop, no import needed either way, works identically under jCardSim and on real hardware:

```java
private static void copyBytes(byte[] src, int srcOff, byte[] dst, int dstOff, int len) {
    for (int i = 0; i < len; i++) {
        dst[(short) (dstOff + i)] = src[(short) (srcOff + i)];
    }
}
```

## How these three interact, in the order you'll actually hit them

Building a real CAP from code that has all three issues surfaces them **one at a time**, not all at once -- the converter stops at the first hard error it hits per compilation unit, so you fix one, rebuild, and the next one appears. Don't assume a clean build after fixing the first error means the rest are fine; rebuild after every fix.

## Verifying a fix generalizes, not just "works for my one applet"

If you're fixing this in a **codegen template** (not a single hand-written applet), verify the fix against the codegen's own test suite (golden-file comparisons), not just by rebuilding your one example CAP. A codegen template can have an architectural invariant your fix accidentally violates -- e.g. `javacard-rpc`'s own test suite explicitly asserts the generated skeleton contains **zero** `javacard.framework` imports (`TestGenerateJavaSkeletonCounterTransportShape`'s forbidden-fragment check), which is exactly why `Util.arrayCopyNonAtomic` was the wrong fix for Rule 3 above despite converting fine -- the test suite catches that, a one-off CAP build does not.
