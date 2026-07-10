# CLAUDE.md

Guidance for AI agents working in this repository.

## What this is

The `jc-testing-tools` agent skill + its two supporting tools: `jc-harness` (Go, T=0-forced PC/SC + raw APDU) and `gp-t0-helper` (Java, GlobalPlatform via GlobalPlatformPro). See [SKILL.md](SKILL.md) for the full content and [references/](references/) for methodology.

## Workflow: use the board for everything here

This repo has its own local `task-board` (`.task-board/`, `task-board.config.json`) -- separate from the board of whatever project you're using this skill *from*. **No work in this repo without a tracked board item first** (create/refine an epic-story-task before implementing, per the [project-management skill](https://github.com/relux-works/skill-project-management)). This applies even to a "small" fix -- especially a small fix, since this repo is meant to stay a reliable dependency other projects pull from without re-verifying it themselves.

```bash
task-board q --format compact 'summary()'
task-board m 'create(type=task, parent=STORY-..., name=..., description="...")'
```

## Go conventions

Follow the [go-testing-tools skill](https://github.com/relux-works/skill-go-testing-tools) for testing conventions/practices when changing `jc-harness`. (That skill's `tuitestkit` library specifically targets bubbletea TUI apps, which `jc-harness` is not -- the relevant part here is its general closed-loop write-test-run-validate discipline and Go testing idioms, not the TUI-specific tooling.)

## Build & test

```bash
# jc-harness (Go)
cd tools/jc-harness
go build -o bin/jc-harness .
go vet ./...

# gp-t0-helper (Java)
cd tools/gp-t0-helper
javac -cp gp.jar -d build GpT0.java

# Reinstall everywhere after any change
scripts/setup.sh
```

**Verify against real hardware before calling a change done.** This repo exists specifically because ad hoc/simulator-only testing missed real bugs (see [references/codegen-jc-classic-compatibility.md](references/codegen-jc-classic-compatibility.md) for three examples found only by building a real CAP and installing it on physical hardware). `go vet`/`javac` passing is necessary, not sufficient.

## SDK dependency policy (applies here too)

If a bug surfaces in `javacard-rpc` (or any other `relux-works` dependency) while working on this repo: fix it in that repo, publish a new version there, then update this repo's reference/example to the fixed version. Do not patch a vendored copy or work around it locally in `jc-testing-tools`.

## Consuming-project boundary

If a **consuming project** (using this skill via `jc-harness`/`gp-t0-helper`) needs a fix or new capability, that change belongs **here**, not hand-patched in the consuming project. Land it on this repo's board, implement, verify against real hardware, then have the consuming project re-run `scripts/setup.sh` to pick it up.
