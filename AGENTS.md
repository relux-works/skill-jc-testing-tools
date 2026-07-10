# Repository Guidelines

Same guidance as [CLAUDE.md](CLAUDE.md), for Codex CLI and other AGENTS.md-reading agents.

## What this is

The `jc-testing-tools` agent skill + `jc-harness` (Go) / `gp-t0-helper` (Java). See [SKILL.md](SKILL.md) and [references/](references/).

## Workflow

Own local `task-board` (`.task-board/`) -- track every change here as a board item first, per the [project-management skill](https://github.com/relux-works/skill-project-management). Follow the [go-testing-tools skill](https://github.com/relux-works/skill-go-testing-tools)'s testing conventions for `jc-harness` changes.

## Build & verify

```bash
cd tools/jc-harness && go build -o bin/jc-harness . && go vet ./...
cd tools/gp-t0-helper && javac -cp gp.jar -d build GpT0.java
scripts/setup.sh   # reinstall everywhere after any change
```

Verify against real hardware before calling a change done -- see [references/codegen-jc-classic-compatibility.md](references/codegen-jc-classic-compatibility.md) for why simulator/compile-only verification has missed real bugs here before.

## Dependency and consumer boundaries

- Bug in `javacard-rpc` or another `relux-works` dependency -> fix upstream there, publish, update the reference here. Never vendor a local patch.
- Consuming project needs a fix/capability -> land it here, verify, let the consumer re-run `scripts/setup.sh`. Never hand-patch inside the consuming project.
