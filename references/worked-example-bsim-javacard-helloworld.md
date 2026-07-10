# Worked example: bsim-javacard-helloworld

A complete, physically-verified run of the full dev cycle this skill is distilled from. Project: `bsim-javacard-helloworld` (part of the `bsim` project's `phys-sim-dev-cycle` epic). Hardware: OMNIKEY 3121 reader, a GlobalPlatform-capable test UICC (JavaCard 3.0.4 runtime).

## The IDL

```toml
# idl/helloworld.toml
[applet]
name = "HelloApplet"
version = "1.0.0"
aid = "F0000000AA01"
cla = 0xB0

[methods.echo]
ins = 0x01
[methods.echo.request]
fields = [{ name = "data", type = "bytes" }]
[methods.echo.response]
fields = [{ name = "data", type = "bytes" }]

[methods.getVersion]
ins = 0x02
[methods.getVersion.response]
fields = [{ name = "major", type = "u8" }, { name = "minor", type = "u8" }]
```

Generate the Java skeleton + Kotlin client:

```bash
jcrpc-gen --java com.bsim.helloworld --kotlin com.bsim.helloworld.client idl/helloworld.toml
```

## The two-class applet split

`HelloApplet extends HelloAppletSkeleton` (generated) -- business logic only, implements `onEcho`/`onGetVersion`. A separate `HelloJCApplet extends javacard.framework.Applet` is the actual installable applet: bridges `process(APDU)` into `HelloApplet`'s generated `dispatch(ins, p1, p2, data)`. This split (business logic extends the generated skeleton; a thin separate class is the real `Applet`) is the standard shape used throughout `javacard-rpc`'s own `counter` example too -- copy it rather than merging the two responsibilities into one class.

## Simulator -> CAP -> physical install -> verify, in order

```bash
# 1. jCardSim (no hardware)
./gradlew test --tests "*HelloAppletTest*"

# 2. Real CAP, targeting the card's actual JC 3.0.4 runtime (found via a 0x6438
#    LOAD failure against the newer default kit first -- see cap-build-toolchain.md)
JDK11=/opt/homebrew/opt/openjdk@11/libexec/openjdk.jdk/Contents/Home
JAVA_HOME=$JDK11 PATH="$JDK11/bin:$PATH" \
  ant -f cap-build.xml -Djckit.name=jc304_kit dist

# 3. Physical install (jc-testing-tools' gp-t0-helper; SCP02/keyVersion/i found
#    via the safe-gp-key-discovery.md method beforehand)
java --add-modules java.smartcardio -cp "build:gp.jar" GpT0 install \
  artifacts/helloworld.cap F0000000AA F0000000AA01 F0000000AA01 \
  <kic> <kid> <kik> 20 SCP02 15

# 4. Verify with the real generated typed client, not raw APDU bytes
jc-harness smoke --reader OMNIKEY --aid F0000000AA01 --apdu B0020000
```

## The physical-card bridge daemon (bsim-pcsc-bridge-go)

For an Android/iOS dev loop, `javacard-rpc-client-kotlin`'s `TCPTransport`/`javacard-rpc-client-swift`'s equivalent point at a TCP bridge instead of talking PC/SC directly (an emulator can't reach a USB reader; iOS apps can't reach one at all without MFi certification). `bsim-pcsc-bridge-go` implements the exact same wire protocol as the reference Java bridge in `javacard-rpc` (`FrameCodec`/`MessageType`, byte-for-byte), backed by a real physical card instead of jCardSim, so the same client library works against either backend with zero client-side changes.

Two bugs specific to that daemon, worth knowing if you build something similar:

- **`ebfe/scard`'s raw `Transmit` does not auto-chase T=0 procedure bytes.** `javax.smartcardio` resolves `61xx` ("more data, call GET RESPONSE") and `6Cxx` ("wrong Le, retry with this one") transparently; a raw Go PC/SC binding does not. A response can come back as a bare 2-byte `61 04` instead of the actual echoed data. Fix: chase both procedure bytes explicitly after every `Transmit`, bounded to a handful of hops as a sanity guard.
- **Don't grab the exclusive PC/SC handle eagerly on TCP accept.** A real reader only supports one exclusive session at a time; if the daemon creates a session immediately when a client connects (before the client has sent a single protocol frame), a plain liveness probe (bare connect-then-disconnect) can race and starve out the very next real connection with a spurious "Sharing violation." Create the session lazily, on the first real protocol message -- and never for a pure liveness `PING`, which shouldn't touch hardware at all.

## SDK/dependency hygiene

If you find a bug in `javacard-rpc` (or any `relux-works` library) while working through this cycle: **fix it in that library's own repo and publish a new version. Do not patch a local clone or vendor a workaround into your project.** `javacard-rpc` v0.1.1 exists because of exactly this -- 3 real codegen bugs (see [codegen-jc-classic-compatibility.md](codegen-jc-classic-compatibility.md)) found while building this example, fixed upstream, not patched locally.
