#!/usr/bin/env bash
# Run this ONCE from your repo root on your Mac
# Sets up branch, .gitignore, and the pre-commit safety hook

set -euo pipefail
REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo ".")
cd "$REPO_ROOT"

echo ""
echo "=== Agent Sandbox Runtime — P5 Git Workflow Setup ==="
echo ""

# ── 1. Make sure we're on the right branch ───────────────────
CURRENT=$(git branch --show-current)
if [ "$CURRENT" != "p5/viewer" ]; then
  echo "Switching to p5/viewer branch..."
  git fetch origin 2>/dev/null || true
  git checkout p5/viewer 2>/dev/null || git checkout -b p5/viewer
else
  echo "Already on p5/viewer ✓"
fi

# ── 2. Add entries to .gitignore ─────────────────────────────
GITIGNORE="$REPO_ROOT/.gitignore"
touch "$GITIGNORE"

add_if_missing() {
  local entry="$1"
  local comment="$2"
  if ! grep -qF "$entry" "$GITIGNORE"; then
    echo "" >> "$GITIGNORE"
    echo "# $comment" >> "$GITIGNORE"
    echo "$entry" >> "$GITIGNORE"
    echo "Added to .gitignore: $entry"
  else
    echo "Already in .gitignore: $entry ✓"
  fi
}

add_if_missing "context.md"                    "P5: personal session context — never commit"
add_if_missing "viewer/server/node_modules/"   "P5: Node.js deps"
add_if_missing "viewer/viewer-app/node_modules/" "P5: React deps"
add_if_missing "viewer/viewer-app/dist/"       "P5: React build output"
add_if_missing ".env"                          "API keys and secrets"
add_if_missing ".env.local"                    "Local env overrides"
add_if_missing ".DS_Store"                     "macOS metadata"

# ── 3. Install pre-commit hook ────────────────────────────────
HOOKS_DIR="$REPO_ROOT/.git/hooks"
PRE_COMMIT="$HOOKS_DIR/pre-commit"

cat > "$PRE_COMMIT" << 'HOOK'
#!/usr/bin/env bash
# Pre-commit safety hook — installed by P5 setup script
# Blocks commits containing files that should never be committed

BLOCKED_FILES=("context.md" ".env" ".env.local")
FOUND=()

for f in "${BLOCKED_FILES[@]}"; do
  if git diff --cached --name-only | grep -qF "$f"; then
    FOUND+=("$f")
  fi
done

if [ ${#FOUND[@]} -gt 0 ]; then
  echo ""
  echo "  COMMIT BLOCKED — the following files must not be committed:"
  for f in "${FOUND[@]}"; do
    echo "    ✗  $f"
  done
  echo ""
  echo "  Run: git restore --staged ${FOUND[*]}"
  echo "  Then commit again."
  echo ""
  exit 1
fi

# Also warn if committing outside viewer/ folder
NON_VIEWER=$(git diff --cached --name-only | grep -v "^viewer/" | grep -v "^CLAUDE.md" | grep -v "^\.gitignore" || true)
if [ -n "$NON_VIEWER" ]; then
  echo ""
  echo "  WARNING: You are committing files outside viewer/ :"
  echo "$NON_VIEWER" | sed 's/^/    /'
  echo ""
  echo "  Are you sure? (yes to continue, anything else to cancel)"
  read -r ANSWER < /dev/tty
  if [ "$ANSWER" != "yes" ]; then
    echo "  Commit cancelled."
    exit 1
  fi
fi

exit 0
HOOK

chmod +x "$PRE_COMMIT"
echo "Pre-commit hook installed ✓"

# ── 4. Create context.md if it doesn't exist ─────────────────
if [ ! -f "$REPO_ROOT/context.md" ]; then
  echo "Creating context.md..."
  cat > "$REPO_ROOT/context.md" << 'CONTEXT'
# P5 Context — update at end of every session

## Session log
<!-- Add a new entry each session: date, what you did, what's next -->

### [DATE]
- Did:
- Next:
- Blockers:

## Current week: Week 2

## Progress
- [x] GitHub repo, VM, Notion, static demo
- [ ] WebSocket server
- [ ] React dashboard layout
- [ ] Connect WebSocket to React UI
CONTEXT
  echo "context.md created ✓"
else
  echo "context.md already exists ✓"
fi

# ── 5. Verify context.md is ignored ──────────────────────────
if git check-ignore -q context.md 2>/dev/null; then
  echo "Verified: context.md is ignored by git ✓"
else
  echo "WARNING: context.md may not be properly ignored — check .gitignore"
fi

# ── 6. Print summary ─────────────────────────────────────────
echo ""
echo "================================================"
echo " Setup complete!"
echo ""
echo " Branch  : $(git branch --show-current)"
echo " Hook    : .git/hooks/pre-commit installed"
echo " Ignored : context.md, node_modules, .env, dist"
echo ""
echo " Daily workflow:"
echo "   Mac terminal 1: export ANTHROPIC_API_KEY=... && claude"
echo "   Mac terminal 2: cd repo && git status"
echo "   Lima VM:        limactl shell agentsandbox && node viewer/server/server.js"
echo "================================================"
echo ""
