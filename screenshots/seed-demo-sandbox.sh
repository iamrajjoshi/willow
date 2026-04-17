#!/usr/bin/env bash
# Build a self-contained $HOME for VHS recording. Isolates willow state from
# the recorder's real repos so the tapes show fictional branches/paths only.
#
# Usage: source this script (not execute) so $HOME stays exported to the tape.
#   source /path/to/seed-demo-sandbox.sh
set -euo pipefail

DEMO_HOME="${DEMO_HOME:-/tmp/wwdemo}"
WILLOW_BIN="${WILLOW_BIN:-/Users/raj.joshi/.willow/worktrees/willow/raj--chore--audit-cleanup/bin/willow}"

rm -rf "$DEMO_HOME"
mkdir -p "$DEMO_HOME" "$DEMO_HOME/bin"
export HOME="$DEMO_HOME"

# Stub `claude` so `willow dispatch` doesn't run the real agent (which would
# prompt for API keys and leak them into the recording).
cat > "$DEMO_HOME/bin/claude" <<'STUB'
#!/usr/bin/env bash
echo
echo "claude code launched with prompt: $*"
STUB
chmod +x "$DEMO_HOME/bin/claude"
export PATH="$DEMO_HOME/bin:$PATH"

# Don't leak any real API credentials into the demo environment.
unset ANTHROPIC_API_KEY ANTHROPIC_BASE_URL CLAUDE_CODE_ENTRYPOINT

mkdir -p "$HOME/.willow/repos" "$HOME/.willow/worktrees" "$HOME/.willow/status"

# Upstream "remote" holds the demo repo's shared branches.
UPSTREAM="$HOME/upstream/willow.git"
git init --quiet --bare -b main "$UPSTREAM"

# Stage a seed commit in a scratch working tree, then push to the upstream
# so branches exist as origin/* refs on the bare repo willow clones from.
SEED="$HOME/seed"
git init --quiet -b main "$SEED"
(
  cd "$SEED"
  git config user.email "demo@willow.dev"
  git config user.name "willow demo"
  printf "# willow demo\n" > README.md
  git add README.md
  git commit --quiet -m "init"
  git remote add origin "$UPSTREAM"
  git push --quiet origin main
  for b in feat-auth feat-ui feat-dark-mode fix-perf fix-api-timeout; do
    git checkout --quiet -b "$b"
    printf "work on %s\n" "$b" >> "$b.md"
    git add "$b.md"
    git commit --quiet -m "$b: progress"
    git push --quiet origin "$b"
    git checkout --quiet main
  done
)

# Clone the willow repo into the fake $HOME/.willow. This gives us a real
# bare repo at ~/.willow/repos/willow.git plus the default main worktree.
"$WILLOW_BIN" clone "$UPSTREAM" willow > /dev/null 2>&1

# Add the demo worktrees so `willow ls` and the dashboard have content.
# Use `checkout` — branches already exist as origin/* refs after the clone fetch.
for b in feat-auth feat-ui feat-dark-mode fix-perf fix-api-timeout; do
  (cd "$HOME/.willow/worktrees/willow/main" && "$WILLOW_BIN" checkout "$b" --no-fetch > /dev/null 2>&1) || true
done

# Build a stacked trio (parent → child → grandchild) so `willow stack status`
# has something to show.
(
  cd "$HOME/.willow/worktrees/willow/main"
  "$WILLOW_BIN" new auth-refactor --no-fetch > /dev/null 2>&1 || true
  (cd "$HOME/.willow/worktrees/willow/auth-refactor" \
    && "$WILLOW_BIN" new auth-refactor-tests --base auth-refactor --no-fetch > /dev/null 2>&1) || true
  (cd "$HOME/.willow/worktrees/willow/auth-refactor-tests" \
    && "$WILLOW_BIN" new auth-refactor-docs --base auth-refactor-tests --no-fetch > /dev/null 2>&1) || true
)

# Seed fake agent status files so the dashboard shows active/busy sessions.
STATUS_DIR="$HOME/.willow/status/willow"
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
AGO_1M=$(date -u -v-1M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '1 minute ago' +%Y-%m-%dT%H:%M:%SZ)
AGO_3M=$(date -u -v-3M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '3 minutes ago' +%Y-%m-%dT%H:%M:%SZ)
AGO_8M=$(date -u -v-8M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '8 minutes ago' +%Y-%m-%dT%H:%M:%SZ)

mkdir -p "$STATUS_DIR/main"
cat > "$STATUS_DIR/main/sess-abc123.json" <<EOF
{"status":"BUSY","tool":"Edit","session_id":"sess-abc123","timestamp":"$NOW","worktree":"main"}
EOF

mkdir -p "$STATUS_DIR/feat-auth"
cat > "$STATUS_DIR/feat-auth/sess-jkl012.json" <<EOF
{"status":"WAIT","session_id":"sess-jkl012","timestamp":"$AGO_1M","worktree":"feat-auth"}
EOF

mkdir -p "$STATUS_DIR/feat-ui"
cat > "$STATUS_DIR/feat-ui/sess-ghi789.json" <<EOF
{"status":"DONE","session_id":"sess-ghi789","timestamp":"$AGO_3M","worktree":"feat-ui"}
EOF

mkdir -p "$STATUS_DIR/feat-dark-mode"
cat > "$STATUS_DIR/feat-dark-mode/sess-pqr678.json" <<EOF
{"status":"DONE","session_id":"sess-pqr678","timestamp":"$AGO_3M","worktree":"feat-dark-mode"}
EOF
cat > "$STATUS_DIR/feat-dark-mode/sess-stu901.json" <<EOF
{"status":"WAIT","session_id":"sess-stu901","timestamp":"$AGO_1M","worktree":"feat-dark-mode"}
EOF

mkdir -p "$STATUS_DIR/fix-perf"
cat > "$STATUS_DIR/fix-perf/sess-mno345.json" <<EOF
{"status":"BUSY","tool":"Bash","session_id":"sess-mno345","timestamp":"$AGO_1M","worktree":"fix-perf"}
EOF

mkdir -p "$STATUS_DIR/fix-api-timeout"
cat > "$STATUS_DIR/fix-api-timeout/sess-vwx234.json" <<EOF
{"status":"DONE","session_id":"sess-vwx234","timestamp":"$AGO_8M","worktree":"fix-api-timeout"}
EOF

echo "sandbox at $HOME"
