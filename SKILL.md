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

The client side has its own cheap-to-expensive progression: the same generated
jcrpc client runs against three targets by swapping its `APDUTransport` --
**simulator** (`TCPTransport` -> bridge -> jCardSim), **reader**
([`OmnikeyTransport`](https://github.com/relux-works/jc-omnikey-transport) ->
real card in a PC/SC reader, host JVM, no phone), then **device**
(`OmapiTransport` -> card in a phone slot, the only lane that exercises OMAPI
access control). The reader lane is the one that lets you hit real converter /
Security Domain framing / card quirks right after applet development, before the
human-gated phone lane. See [references/transport-lanes.md](references/transport-lanes.md).

---

## Use the javacard-rpc family for applet contracts

Don't hand-roll CLA/INS dispatch and hand-write client-side APDU construction. `relux-works/javacard-rpc` generates both sides from one TOML IDL:

- **`javacard-rpc`** -- the IDL + Go codegen (`jcrpc-gen`). One `.toml` file defines the applet's methods (INS codes, request/response fields); codegen produces a Java server skeleton and typed Kotlin/Swift clients from it.
- **`javacard-rpc-server-javacard`** -- `AppletBase`, the server-side runtime the generated skeleton builds on.
- **`javacard-rpc-client-kotlin`** -- `APDUCommand`/`APDUResponse`/`APDUTransport`/`TCPTransport`, the Kotlin/JVM client runtime. `TCPTransport` is plain `java.net.Socket` -- it runs unmodified on Android, no separate "Android transport" needed.
- **`jc-omnikey-transport`** -- an `APDUTransport` backed by a physical PC/SC reader (`javax.smartcardio`, T=0 forced). Swap it under the generated client to run the real client against a real card in a reader from a host JVM -- no bridge, no phone. This is the "reader" lane; see [references/transport-lanes.md](references/transport-lanes.md).
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

# Read the two standard classic-GSM identity EFs. The command warm-resets the
# card, uses one T=0 session, validates the file-operation status words, and
# applies the distinct ICCID/IMSI BCD rules (including EF_IMSI's length and
# type/parity nibble). The output contains subscriber identifiers: consume it
# in memory and never redirect it to shared logs or task evidence.
jc-harness sim-meta --reader OMNIKEY
# {"reader": "...", "protocol": "T=0", "iccid": "...", "imsi": "..."}

# seq: send a raw APDU sequence over ONE session with no implicit SELECT --
# the generic stateful primitive smoke specializes. Selection/file-system
# state persists across APDUs, so this is what you use to read a card's
# non-standard classic-GSM file system or any raw-read-then-reselect-AID
# provisioning flow that a leading AID SELECT would break. Use `sim-meta`, not
# hand-written APDUs, for EF_ICCID / EF_IMSI. (`apdu` can't be chained for this
# -- it reconnects per call and loses selection state.)
jc-harness seq --reader OMNIKEY --apdu A0A4000002 3F00,A0A4000002 7F20,A0A4000002 6F07,A0B0000009
# {"reader": "...", "results": [{"sw": "9f0f", "data": ""}, ..., {"sw": "9000", "data": "08..."}]}
```

### Read `EF_MSISDN` safely (classic SIM)

`EF_MSISDN` is a record-based, optional on-card copy of a telephone number. It
is not the operator's source of truth: it can be absent, blank, stale, or have
multiple records. Keep the whole exchange in one `seq` session and identify the
card first (for example by `EF_ICCID`) before associating a result with metadata.

```bash
# MF -> DF_TELECOM -> EF_MSISDN -> READ RECORD 1, absolute mode.
# The test UICC required a 28-byte record, so Le is 0x1C here.
jc-harness seq --reader OMNIKEY --reset --apdu \
  A0A40000023F00,A0A40000027F10,A0A40000026F40,A0B201041C
```

`9Fxx` from either `SELECT` is a successful selection with response bytes
available. Do not assume `0x1C` for another card: obtain the record length from
the FCP or card documentation. On the verified test UICC, `READ RECORD ...
Le=00` returned `671C` and a retry with `Le=1C` succeeded; that is an empirical
card-specific observation, not a general `67xx` decoding rule.

The returned record has the ADN layout: alpha identifier, BCD-number length,
TON/NPI, BCD digits, capability/configuration bytes, and an extension-record
reference. Decode only the declared BCD-number length; if the extension
reference is present, read the appropriate extension record before treating the
number as complete. An all-`FF` record, or an empty number-length field, means
that `EF_MSISDN` is selected but not provisioned.

MSISDN is subscriber data. Do not print raw command JSON, redirect it to logs,
or put the number in docs/task evidence. Decode and compare it in memory, then
report only a redacted result. A PC/SC reader-side read also does **not** grant
an installed applet access: the applet still needs the applicable UICC file-view
and access-domain permission for `EF_MSISDN`.

### USSD is terminal/network work, not reader APDU

A PC/SC reader has an APDU transport to the UICC, but no cellular modem, radio
registration, or network signalling stack. It therefore cannot originate an
operator USSD session or receive the network response merely by sending an APDU
to the card.

An STK/USAT applet can issue the proactive `SEND USSD` command, but the Mobile
Equipment (ME) executes that command against the network and returns the result
in its Terminal Response. The reader lane cannot substitute for that ME: even a
host that emulates parts of the STK terminal protocol has no mobile-network
attachment. Use this flow only with the SIM in a registered handset or cellular
modem that supports the required STK terminal profile.

For host-driven carrier USSD automation, put the SIM in a registered modem or
phone and use that device's telephony/modem interface; parse the result there.
Use `jc-harness` only for the reader-side APDU, filesystem, and applet-debugging
work. Treat carrier USSD as an external action: do not send a code while merely
probing a card, and do not log a returned subscriber number.

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

# Guarded migration/rollback automation: authenticated and idempotent, but
# only the precise GP not-found status 6A88 is accepted as already absent.
# Every other delete/authentication/transport failure exits non-zero.
java --add-modules java.smartcardio -cp "build:gp.jar" GpT0 delete-if-present <aidHex> <kic> <kid> <kik> <keyVersionHex> <scpName> <iHex>
```

**Finding the right key/SCP/version/i combination without risking the card:** [references/safe-gp-key-discovery.md](references/safe-gp-key-discovery.md) documents the exact safe probing method (`trysc`) -- every combination attempt is provably risk-free until the correct one is found, because the local cryptogram check fails before `EXTERNAL AUTHENTICATE` is ever sent for a wrong guess.

---

## `android-omapi-probe.sh`: Android OMAPI readiness

The final dev-cycle stage -- talking to your installed applet from a real Android app
over OMAPI (`android.se.omapi`) instead of the host PC/SC reader -- has device- and
card-dependent failure modes that are non-obvious and eat real time. `scripts/android-omapi-probe.sh`
diagnoses them from the host over `adb`.

```bash
# static readiness (no app needed): API>=28, .uicc feature, which slot has the card
scripts/android-omapi-probe.sh                 # auto-pick the single physical device
scripts/android-omapi-probe.sh --serial <sn>
# tail live OMAPI signals while your app/test runs: getReader / isSecureElementPresent / AccessControlEnforcer
scripts/android-omapi-probe.sh --serial <sn> --watch
```

Two barriers show up here, in order, each with a distinct error (full detail in
[references/android-omapi-readiness.md](references/android-omapi-readiness.md)):

1. **Card in the wrong SIM slot** -> `No OMAPI reader with a secure element is available`.
   Most vendor firmware exposes only the `SIM1` OMAPI reader regardless of the card's
   slot; move the card to slot 1. (OMAPI *can* address slots by reader name `SIM1`/`SIM2`,
   but exposing a per-slot reader is vendor-optional -- it's a firmware gap, not your app.)
2. **On-card access control (SEAC), no rules** -> `AccessControlEnforcer: No ARF exists` ->
   `Deny any access`. The channel physically opens, then Android denies because the card
   carries no ARA-M/ARF access rule for your app's signing cert. Fix is **card-side**:
   provision an ARA-M applet with a grant rule (via `gp-t0-helper`), not an app change.

This is host-side `adb` tooling on purpose -- it is deliberately a standalone script,
not part of the PC/SC-only `jc-harness` Go binary.

---

## References

| File | Covers |
|---|---|
| [references/worked-example-bsim-javacard-helloworld.md](references/worked-example-bsim-javacard-helloworld.md) | Full physically-verified walkthrough: simulator -> CAP -> install -> typed client, with the exact commands |
| [references/methodology.md](references/methodology.md) | What each dev-cycle stage actually verifies, and what it doesn't |
| [references/transport-lanes.md](references/transport-lanes.md) | The three client transport lanes (simulator / reader / device), which bug class each catches, and the swap |
| [references/cos-export-applet-what-lives-where.md](references/cos-export-applet-what-lives-where.md) | What lives where — the COS library (in firmware) vs export files (build machine only) vs your applet (you load it); the two linkings; and the SDK-vs-entitlement mapping. Read this first |
| [references/applet-file-access-access-domain.md](references/applet-file-access-access-domain.md) | How an applet reads the card's own files (FileView / Path A): the Access Domain grant, keys vs grant, who installs it, and why it usually falls back to host-read provisioning |
| [references/ios-sim-applet-bip-messaging.md](references/ios-sim-applet-bip-messaging.md) | iOS has no APDU path to the SIM (no OMAPI/entitlement); the backend-hub + BIP async bidirectional messaging pattern that reaches the applet below the OS, its privilege model, and what our keys cover vs what to request |
| [references/t0-vs-t1.md](references/t0-vs-t1.md) | Why T=0 must be forced explicitly, and what breaks if it isn't |
| [references/safe-gp-key-discovery.md](references/safe-gp-key-discovery.md) | Non-destructive method for finding the correct GP key/SCP/version/i combination |
| [references/gp-t0-driver-pattern.md](references/gp-t0-driver-pattern.md) | Why GlobalPlatformPro's CLI can't be used directly, and the library-mode workaround |
| [references/codegen-jc-classic-compatibility.md](references/codegen-jc-classic-compatibility.md) | Exact rules generated/hand-written applet code must follow to convert on real Java Card Classic |
| [references/cap-build-toolchain.md](references/cap-build-toolchain.md) | `ant-javacard` setup, JDK-version-per-kit gotchas, `ints="true"` |
| [references/android-omapi-readiness.md](references/android-omapi-readiness.md) | Android OMAPI readiness: feature flags, SIM-slot exposure reality, and the wrong-slot + on-card-access-control barriers (with `android-omapi-probe.sh`) |

---

## Working on this skill/tool itself

This repo tracks its own work on a local [project-management](https://github.com/relux-works/skill-project-management) task-board (`.task-board/`) -- not the board of whatever project you're using this skill from. If you extend `jc-harness` or `gp-t0-helper`, or need to fix a methodology doc, do it here: create/update a board item, make the change, run `scripts/setup.sh` to reinstall, and verify against real hardware before calling it done. See [go-testing-tools](https://github.com/relux-works/skill-go-testing-tools) for Go testing practices/conventions to follow for `jc-harness` changes.

**Consuming projects should extend this skill, not accumulate local patches.** If a project working with `jc-harness`/`gp-t0-helper` needs a fix or a new capability, that belongs here, released properly, and picked up by re-running `scripts/setup.sh` -- not hand-patched inside the consuming project's own repo.
