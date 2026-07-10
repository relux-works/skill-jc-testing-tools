---
name: jc-testing-tools
description: >
  Physical JavaCard/UICC dev-cycle toolkit: a Go harness (jc-harness) for
  T=0-forced PC/SC reader access and raw APDU exchange, a Java helper
  (gp-t0-helper) for GlobalPlatform install/delete/secure-channel key
  discovery, and the javacard-rpc family (IDL codegen + Kotlin/Swift/Java
  runtimes) as the recommended applet-contract pattern. Distilled from a real
  simulator-to-physical-hardware dev cycle (bsim-javacard-helloworld,
  bsim-pcsc-bridge-go).
triggers:
  - javacard
  - java card
  - uicc
  - physical sim dev
  - apdu
  - globalplatform
  - global platform
  - jcardsim
  - javacard-rpc
  - jc-rpc
  - pc/sc reader
  - pcsc reader
  - t=0 t=1
  - джавакард
  - джава кард
  - физическая симка
  - физ симка
  - глобалплатформ
---

# JC Testing Tools

Toolkit and methodology for the physical Java Card/UICC dev cycle: write an applet, verify it in a simulator, build a real CAP, install it on physical hardware via GlobalPlatform, and talk to it over APDU -- reproducibly, without rediscovering the same PC/SC and GlobalPlatform gotchas every time.

Everything here was distilled from a real end-to-end session (see [references/worked-example-bsim-javacard-helloworld.md](references/worked-example-bsim-javacard-helloworld.md)) that went from "nothing talks to the reader" through a full loop, including several genuinely nasty, non-obvious bugs. This skill exists so the next project doesn't have to hit them again.

---

## Prerequisites

- **Go 1.21+** (jc-harness)
- **Java 17+** for `gp-t0-helper` itself; **Java 11 specifically** if you also need to build a CAP targeting a JavaCard 3.0.4 kit via `ant-javacard` (see [references/cap-build-toolchain.md](references/cap-build-toolchain.md))
- **A PC/SC reader** and a GlobalPlatform-capable test UICC. A locked production/operator SIM will not work for CAP install.
- `ant` + `ant-javacard` + an Oracle JavaCard SDK kit for building CAP files (bootstrap pattern in references)

---

## The dev cycle

```
applet source
  -> jCardSim simulator test (no hardware)
  -> real CAP build (ant-javacard, targetsdk matching the card's actual runtime)
  -> physical GlobalPlatform install (jc-harness + gp-t0-helper, T=0 forced)
  -> APDU verify (jc-harness smoke, or a javacard-rpc-generated typed client)
```

Each stage is a real, separate verification step -- a CAP that builds does not mean it converts for real hardware; a CAP that installs does not mean the applet's runtime logic is bug-free; jCardSim passing does not mean the real JC converter will accept the same code. See [references/methodology.md](references/methodology.md) for exactly which failure modes appear at which stage and how to tell them apart.

---

## Use the javacard-rpc family for applet contracts

Don't hand-roll CLA/INS dispatch and hand-write client-side APDU construction. `relux-works/javacard-rpc` generates both sides from one TOML IDL:

- **`javacard-rpc`** -- the IDL + Go codegen (`jcrpc-gen`). One `.toml` file defines the applet's methods (INS codes, request/response fields); codegen produces a Java server skeleton and typed Kotlin/Swift clients from it.
- **`javacard-rpc-server-javacard`** -- `AppletBase`, the server-side runtime the generated skeleton builds on.
- **`javacard-rpc-client-kotlin`** -- `APDUCommand`/`APDUResponse`/`APDUTransport`/`TCPTransport`, the Kotlin/JVM client runtime. `TCPTransport` is plain `java.net.Socket` -- it runs unmodified on Android, no separate "Android transport" needed.
- **`javacard-rpc-client-swift`** -- the same shape on Swift/iOS.

### Worked example

`bsim-javacard-helloworld` (see the full writeup in [references/worked-example-bsim-javacard-helloworld.md](references/worked-example-bsim-javacard-helloworld.md)) is a complete, physically-verified example of this pattern:

- `idl/helloworld.toml` -- the IDL contract (`echo`, `getVersion`)
- `HelloApplet.java` / `HelloJCApplet.java` -- generated skeleton subclass + the thin `javacard.framework.Applet` DI wrapper (this two-class split -- business logic extends the generated skeleton, a separate tiny class is the actual installable `Applet` -- is the standard shape; copy it, don't reinvent it)
- `DaemonSmokeTest.kt` -- the *real* `javacard-rpc-client-kotlin` `TCPTransport`, talking through a physical-card-backed bridge daemon to the applet on real hardware, via the generated typed `HelloAppletClient`

That project also found and fixed **3 real bugs in `javacard-rpc`'s Java codegen** that only surface when converting a real CAP (jCardSim tolerates all of them, since it's a real JVM): String-based exceptions, `int`-typed array indexing, and `System.arraycopy` (not part of the real JC Classic `java.lang.System` stub). Fixed upstream in `javacard-rpc` v0.1.1 -- if you hit a codegen bug against real hardware, **fix it in `javacard-rpc` and publish a new version, don't patch your local copy.** See [references/codegen-jc-classic-compatibility.md](references/codegen-jc-classic-compatibility.md) for the exact rules the generated skeleton must follow to convert on real Java Card Classic.

---

## `jc-harness` (Go): PC/SC-native operations

Handles everything that doesn't need GlobalPlatform secure-channel crypto: reader discovery, T=0-forced connect, raw APDU exchange, APDU smoke testing.

```bash
jc-harness readers
# {"readers": ["OMNIKEY AG Smart Card Reader USB"]}

jc-harness atr --reader OMNIKEY
# {"reader": "...", "atr": "3b9f9680..."}

jc-harness smoke --reader OMNIKEY --aid F0000000AA01 --apdu B0010000...,B0020000
# {"reader": "...", "select": {"sw": "9000", "data": ""}, "results": [{"sw": "9000", "data": "..."}, ...]}
```

Every command prints one JSON object/array to stdout, success or `{"error": "..."}` on failure -- no flag has a default, missing a required flag is a hard, specific error. See `tools/jc-harness/main.go`'s package doc for the full design rationale (including why this does *not* adopt the full `agent-facing-api` query-DSL pattern -- there's no multi-entity dataset here to project/filter against, just a handful of imperative hardware actions).

**Why Go, and why T=0 is forced explicitly:** see [references/t0-vs-t1.md](references/t0-vs-t1.md). Short version: letting the OS negotiate ("any" protocol) can pick T=1 on a reader/card that formally advertises both in its ATR but only actually works over T=0, and then every APDU fails with `SCARD_E_NOT_TRANSACTED` -- forcing T=0 explicitly at connect time is the fix, and it's not optional/best-effort, it's required for this class of hardware.

---

## `gp-t0-helper` (Java): GlobalPlatform operations

GlobalPlatform secure-channel crypto (SCP02/SCP03 session-key derivation, `INITIALIZE UPDATE`/`EXTERNAL AUTHENTICATE`, MAC/ENC) is **not** reimplemented here. It shells out to [GlobalPlatformPro](https://github.com/martinpaljak/GlobalPlatformPro) (`gp.jar`, bundled), driven through its own public library classes (`GPSession`, `PlaintextKeys`) with an explicit T=0-forced `javax.smartcardio` connection underneath -- GlobalPlatformPro's own CLI has no way to force the protocol, which is the actual reason this helper exists. See [references/gp-t0-driver-pattern.md](references/gp-t0-driver-pattern.md) for exactly why and how.

```bash
# Safe, non-destructive: never sends EXTERNAL AUTHENTICATE if the local
# cryptogram check doesn't match -- use this to find the right key/SCP/i
# combination before ever risking a real attempt.
java --add-modules java.smartcardio -cp "build:gp.jar" GpT0 trysc <kic> <kid> <kik> <keyVersionHex> <scpName> <iHex>

java --add-modules java.smartcardio -cp "build:gp.jar" GpT0 install <cap> <pkgAid> <appletAid> <instanceAid> <kic> <kid> <kik> <keyVersionHex> <scpName> <iHex>

java --add-modules java.smartcardio -cp "build:gp.jar" GpT0 delete <aidHex> <kic> <kid> <kik> <keyVersionHex> <scpName> <iHex>
```

**Finding the right key/SCP/version/i combination without risking the card:** [references/safe-gp-key-discovery.md](references/safe-gp-key-discovery.md) documents the exact safe probing method (`trysc`) -- every combination attempt is provably risk-free until the correct one is found, because the local cryptogram check fails before `EXTERNAL AUTHENTICATE` is ever sent for a wrong guess.

---

## References

| File | Covers |
|---|---|
| [references/worked-example-bsim-javacard-helloworld.md](references/worked-example-bsim-javacard-helloworld.md) | Full physically-verified walkthrough: simulator -> CAP -> install -> typed client, with the exact commands |
| [references/methodology.md](references/methodology.md) | What each dev-cycle stage actually verifies, and what it doesn't |
| [references/t0-vs-t1.md](references/t0-vs-t1.md) | Why T=0 must be forced explicitly, and what breaks if it isn't |
| [references/safe-gp-key-discovery.md](references/safe-gp-key-discovery.md) | Non-destructive method for finding the correct GP key/SCP/version/i combination |
| [references/gp-t0-driver-pattern.md](references/gp-t0-driver-pattern.md) | Why GlobalPlatformPro's CLI can't be used directly, and the library-mode workaround |
| [references/codegen-jc-classic-compatibility.md](references/codegen-jc-classic-compatibility.md) | Exact rules generated/hand-written applet code must follow to convert on real Java Card Classic |
| [references/cap-build-toolchain.md](references/cap-build-toolchain.md) | `ant-javacard` setup, JDK-version-per-kit gotchas, `ints="true"` |

---

## Working on this skill/tool itself

This repo tracks its own work on a local [project-management](https://github.com/relux-works/skill-project-management) task-board (`.task-board/`) -- not the board of whatever project you're using this skill from. If you extend `jc-harness` or `gp-t0-helper`, or need to fix a methodology doc, do it here: create/update a board item, make the change, run `scripts/setup.sh` to reinstall, and verify against real hardware before calling it done. See [go-testing-tools](https://github.com/relux-works/skill-go-testing-tools) for Go testing practices/conventions to follow for `jc-harness` changes.

**Consuming projects should extend this skill, not accumulate local patches.** If a project working with `jc-harness`/`gp-t0-helper` needs a fix or a new capability, that belongs here, released properly, and picked up by re-running `scripts/setup.sh` -- not hand-patched inside the consuming project's own repo.
