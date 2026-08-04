# skill-jc-testing-tools

Physical JavaCard/UICC dev-cycle toolkit and agent skill: `jc-harness` (Go, T=0-forced PC/SC + raw APDU), `gp-t0-helper` (Java, GlobalPlatform install/delete/secure-channel key discovery via GlobalPlatformPro), and a methodology writeup for the javacard-rpc family as the recommended applet-contract pattern.

See [SKILL.md](SKILL.md) for the full skill content, and [references/](references/) for the distilled methodology (T=0 vs T=1, safe GP key discovery, Java Card Classic codegen compatibility rules, CAP build toolchain gotchas, a full physically-verified worked example).

## Setup

```bash
scripts/setup.sh
```

Builds both tools, installs `jc-harness`/`gp-t0-helper` to `~/.local/bin`, and installs the skill into `~/.agents/skills/jc-testing-tools` (symlinked from `~/.claude/skills` and `~/.codex/skills`).

## Quick start

```bash
jc-harness readers
jc-harness atr --reader OMNIKEY
jc-harness smoke --reader OMNIKEY --aid <hex> --apdu <hex>[,<hex>...]
jc-harness sim-meta --reader OMNIKEY
# Read-only optional telephone-number record; use the card's actual record length.
jc-harness seq --reader OMNIKEY --reset --apdu A0A40000023F00,A0A40000027F10,A0A40000026F40,A0B201041C

gp-t0-helper trysc <kic> <kid> <kik> <keyVersionHex> <scpName> <iHex>
gp-t0-helper install <cap> <pkgAid> <appletAid> <instanceAid> <kic> <kid> <kik> <keyVersionHex> <scpName> <iHex>
gp-t0-helper secure-apdu <kic> <kid> <kik> <keyVersionHex> <scpName> <iHex> <apduHex> [<apduHex>...]
gp-t0-helper delete-if-present <aidHex> <kic> <kid> <kik> <keyVersionHex> <scpName> <iHex>
```

The `EF_MSISDN` example reads only one record and uses `0x1C` solely because
that is the verified test-card record length. See [SKILL.md](SKILL.md) for FCP
length discovery, ADN-record decoding, redaction, and the distinction between a
reader-side read and applet file access.

`sim-meta` is the read-only canonical path for `EF_ICCID` and `EF_IMSI`: it
resets the card, keeps file selections in one T=0 session, and applies the two
different BCD layouts. Its JSON contains subscriber identifiers, so consume it
in memory and never redirect the output to shared logs.

`jc-harness` does not originate carrier USSD sessions: a PC/SC reader has no
cellular modem or network attachment. Use a registered handset or modem for
USSD, and see [SKILL.md](SKILL.md) for the STK `SEND USSD` boundary.

`secure-apdu` sends the supplied commands in order through GlobalPlatformPro's
authenticated secure-messaging wrapper while retaining the helper's forced
T=0 card connection. It stops at the first response other than `SW=9000`.
Library logging defaults to `warn` so static, diversified, and session keys are
not printed; set the standard `org.slf4j.simpleLogger.defaultLogLevel` JVM
property explicitly when verbose local diagnostics are required.

`delete-if-present` is the fail-closed idempotent delete used by guarded
migration/rollback automation. It opens the same authenticated forced-T=0
Security Domain path and accepts only GP status `6A88` as already absent. A
`6A86`, transport error, authentication failure, or any other `GPException`
remains a non-zero process failure. The legacy `delete` command retains its
historical best-effort output contract for compatibility.

## Development validation

```bash
cd tools/gp-t0-helper
javac -cp gp.jar -d build GpT0.java GpT0Test.java
java -cp "build:gp.jar" GpT0Test
```

The compiled classes stay under `tools/gp-t0-helper/build/`. Physical-card
validation uses the installed `gp-t0-helper` and `jc-harness` commands above.

## Working on this repo

Own local task-board (`.task-board/`), separate from any consuming project's board. See "Working on this skill/tool itself" in [SKILL.md](SKILL.md).
