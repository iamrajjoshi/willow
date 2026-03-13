#!/usr/bin/env bash
# Seeds fake multi-session status data for screenshot/GIF recording
set -euo pipefail

STATUS_DIR="$HOME/.willow/status/willow"

# Clean up any existing test data
rm -rf "$STATUS_DIR/master"
rm -rf "$STATUS_DIR/feat-dark-mode"
rm -rf "$STATUS_DIR/feat-auth"
rm -rf "$STATUS_DIR/payments-v2"
rm -rf "$STATUS_DIR/dashboard-ui"
rm -rf "$STATUS_DIR/api-tests"

NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
AGO_1M=$(date -u -v-1M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '1 minute ago' +%Y-%m-%dT%H:%M:%SZ)
AGO_3M=$(date -u -v-3M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '3 minutes ago' +%Y-%m-%dT%H:%M:%SZ)
AGO_8M=$(date -u -v-8M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '8 minutes ago' +%Y-%m-%dT%H:%M:%SZ)

# master: 2 BUSY sessions (multi-agent)
mkdir -p "$STATUS_DIR/master"
cat > "$STATUS_DIR/master/sess-abc123.json" <<EOF
{"status":"BUSY","tool":"Edit","session_id":"sess-abc123","timestamp":"$NOW","worktree":"main"}
EOF
cat > "$STATUS_DIR/master/sess-def456.json" <<EOF
{"status":"BUSY","tool":"Bash","session_id":"sess-def456","timestamp":"$AGO_1M","worktree":"main"}
EOF

# payments-v2: 1 WAIT session
mkdir -p "$STATUS_DIR/payments-v2"
cat > "$STATUS_DIR/payments-v2/sess-jkl012.json" <<EOF
{"status":"WAIT","session_id":"sess-jkl012","timestamp":"$AGO_1M","worktree":"payments-v2"}
EOF

# dashboard-ui: 1 DONE (unread)
mkdir -p "$STATUS_DIR/dashboard-ui"
cat > "$STATUS_DIR/dashboard-ui/sess-ghi789.json" <<EOF
{"status":"DONE","session_id":"sess-ghi789","timestamp":"$AGO_3M","worktree":"dashboard-ui"}
EOF

# api-tests: 1 DONE (older)
mkdir -p "$STATUS_DIR/api-tests"
cat > "$STATUS_DIR/api-tests/sess-mno345.json" <<EOF
{"status":"DONE","session_id":"sess-mno345","timestamp":"$AGO_8M","worktree":"api-tests"}
EOF

# feat-dark-mode: 1 DONE (unread) + 1 WAIT (kept for status demo)
mkdir -p "$STATUS_DIR/feat-dark-mode"
cat > "$STATUS_DIR/feat-dark-mode/sess-ghi789.json" <<EOF
{"status":"DONE","session_id":"sess-ghi789","timestamp":"$AGO_3M","worktree":"feat-dark-mode"}
EOF
cat > "$STATUS_DIR/feat-dark-mode/sess-jkl012.json" <<EOF
{"status":"WAIT","session_id":"sess-jkl012","timestamp":"$AGO_1M","worktree":"feat-dark-mode"}
EOF

# feat-auth: 1 DONE (unread) (kept for status demo)
mkdir -p "$STATUS_DIR/feat-auth"
cat > "$STATUS_DIR/feat-auth/sess-mno345.json" <<EOF
{"status":"DONE","session_id":"sess-mno345","timestamp":"$AGO_8M","worktree":"feat-auth"}
EOF

echo "Seeded demo data in $STATUS_DIR"
