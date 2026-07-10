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

gp-t0-helper trysc <kic> <kid> <kik> <keyVersionHex> <scpName> <iHex>
gp-t0-helper install <cap> <pkgAid> <appletAid> <instanceAid> <kic> <kid> <kik> <keyVersionHex> <scpName> <iHex>
```

## Working on this repo

Own local task-board (`.task-board/`), separate from any consuming project's board. See "Working on this skill/tool itself" in [SKILL.md](SKILL.md).
