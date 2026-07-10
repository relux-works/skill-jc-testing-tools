#!/usr/bin/env bash
set -euo pipefail

SKILL_NAME="jc-testing-tools"
SKILL_DIR="$(cd "$(dirname "$0")/.." && pwd)"

AGENTS_DIR="$HOME/.agents/skills"
CLAUDE_DIR="$HOME/.claude/skills"
CODEX_DIR="$HOME/.codex/skills"
BIN_DIR="$HOME/.local/bin"

echo "Installing skill: $SKILL_NAME"
echo "  Source: $SKILL_DIR"

# --- 0. Prerequisites -------------------------------------------------------

if ! command -v go >/dev/null 2>&1; then
  echo "error: go is required (jc-harness) and was not found on PATH" >&2
  exit 1
fi
if ! command -v java >/dev/null 2>&1; then
  echo "error: java is required (gp-t0-helper) and was not found on PATH" >&2
  exit 1
fi

# --- 1. Build jc-harness (Go) -----------------------------------------------

echo "  Building jc-harness..."
(
  cd "$SKILL_DIR/tools/jc-harness"
  go build -o bin/jc-harness .
)
mkdir -p "$BIN_DIR"
cp "$SKILL_DIR/tools/jc-harness/bin/jc-harness" "$BIN_DIR/jc-harness"
chmod +x "$BIN_DIR/jc-harness"
echo "  Installed -> $BIN_DIR/jc-harness"

# --- 2. Build gp-t0-helper (Java) and install a wrapper script -------------

echo "  Building gp-t0-helper..."
(
  cd "$SKILL_DIR/tools/gp-t0-helper"
  mkdir -p build
  javac -cp gp.jar -d build GpT0.java
)

cat > "$BIN_DIR/gp-t0-helper" <<EOF
#!/usr/bin/env bash
# Installed by skill-jc-testing-tools/scripts/setup.sh -- do not edit by hand,
# re-run setup.sh after changing tools/gp-t0-helper/GpT0.java instead.
exec java --add-modules java.smartcardio \\
  -cp "$AGENTS_DIR/$SKILL_NAME/tools/gp-t0-helper/build:$AGENTS_DIR/$SKILL_NAME/tools/gp-t0-helper/gp.jar" \\
  GpT0 "\$@"
EOF
chmod +x "$BIN_DIR/gp-t0-helper"
echo "  Installed -> $BIN_DIR/gp-t0-helper (wraps the copy under $AGENTS_DIR/$SKILL_NAME, installed in step 3)"

# --- 3. Copy skill (incl. built tools/gp-t0-helper/build) into .agents/skills/ ---
# rsync excludes .git and setup.sh (the skill copy is not meant to be a git
# checkout you develop against -- develop in $SKILL_DIR, re-run this script).

if [ -L "$AGENTS_DIR/$SKILL_NAME" ]; then
  rm -f "$AGENTS_DIR/$SKILL_NAME"
fi
mkdir -p "$AGENTS_DIR/$SKILL_NAME"
rsync -a --delete "$SKILL_DIR/" "$AGENTS_DIR/$SKILL_NAME/" \
  --exclude='.git' \
  --exclude='.task-board' \
  --exclude='task-board.config.json' \
  --exclude='scripts/setup.sh' \
  --exclude='tools/jc-harness/bin'
echo "  Copied -> $AGENTS_DIR/$SKILL_NAME/"

# --- 4. Symlink from .claude/skills/ and .codex/skills/ ---------------------

mkdir -p "$CLAUDE_DIR"
rm -f "$CLAUDE_DIR/$SKILL_NAME"
ln -s "$AGENTS_DIR/$SKILL_NAME" "$CLAUDE_DIR/$SKILL_NAME"
echo "  Symlink -> $CLAUDE_DIR/$SKILL_NAME"

mkdir -p "$CODEX_DIR"
rm -f "$CODEX_DIR/$SKILL_NAME"
ln -s "$AGENTS_DIR/$SKILL_NAME" "$CODEX_DIR/$SKILL_NAME"
echo "  Symlink -> $CODEX_DIR/$SKILL_NAME"

# --- 5. Verify --------------------------------------------------------------

if [ ! -f "$AGENTS_DIR/$SKILL_NAME/SKILL.md" ]; then
  echo "error: install verification failed -- SKILL.md missing at $AGENTS_DIR/$SKILL_NAME/SKILL.md" >&2
  exit 1
fi
if ! cmp -s "$SKILL_DIR/SKILL.md" "$AGENTS_DIR/$SKILL_NAME/SKILL.md"; then
  echo "error: install verification failed -- installed SKILL.md differs from source" >&2
  exit 1
fi

echo ""
echo "Done."
echo "  jc-harness    -> $(command -v jc-harness 2>/dev/null || echo "$BIN_DIR/jc-harness (add $BIN_DIR to PATH)")"
echo "  gp-t0-helper  -> $(command -v gp-t0-helper 2>/dev/null || echo "$BIN_DIR/gp-t0-helper (add $BIN_DIR to PATH)")"
echo "  skill         -> $AGENTS_DIR/$SKILL_NAME (symlinked into .claude/skills and .codex/skills)"
