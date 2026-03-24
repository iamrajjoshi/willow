#!/usr/bin/env bash
# Seeds fake multi-session status data for screenshot/GIF recording
set -euo pipefail

STATUS_DIR="$HOME/.willow/status/willow"

# Clean up any existing test data
rm -rf "$STATUS_DIR/main"
rm -rf "$STATUS_DIR/feat-auth"
rm -rf "$STATUS_DIR/feat-dark-mode"
rm -rf "$STATUS_DIR/feat-ui"
rm -rf "$STATUS_DIR/fix-perf"
rm -rf "$STATUS_DIR/fix-api-timeout"

NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
AGO_1M=$(date -u -v-1M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '1 minute ago' +%Y-%m-%dT%H:%M:%SZ)
AGO_3M=$(date -u -v-3M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '3 minutes ago' +%Y-%m-%dT%H:%M:%SZ)
AGO_8M=$(date -u -v-8M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '8 minutes ago' +%Y-%m-%dT%H:%M:%SZ)

# main: 2 BUSY sessions (multi-agent)
mkdir -p "$STATUS_DIR/main"
cat > "$STATUS_DIR/main/sess-abc123.json" <<EOF
{"status":"BUSY","tool":"Edit","session_id":"sess-abc123","timestamp":"$NOW","worktree":"main"}
EOF
cat > "$STATUS_DIR/main/sess-def456.json" <<EOF
{"status":"BUSY","tool":"Bash","session_id":"sess-def456","timestamp":"$AGO_1M","worktree":"main"}
EOF

# feat-auth: 1 WAIT session
mkdir -p "$STATUS_DIR/feat-auth"
cat > "$STATUS_DIR/feat-auth/sess-jkl012.json" <<EOF
{"status":"WAIT","session_id":"sess-jkl012","timestamp":"$AGO_1M","worktree":"feat-auth"}
EOF

# feat-ui: 1 DONE (unread)
mkdir -p "$STATUS_DIR/feat-ui"
cat > "$STATUS_DIR/feat-ui/sess-ghi789.json" <<EOF
{"status":"DONE","session_id":"sess-ghi789","timestamp":"$AGO_3M","worktree":"feat-ui"}
EOF

# fix-perf: 1 DONE (older)
mkdir -p "$STATUS_DIR/fix-perf"
cat > "$STATUS_DIR/fix-perf/sess-mno345.json" <<EOF
{"status":"DONE","session_id":"sess-mno345","timestamp":"$AGO_8M","worktree":"fix-perf"}
EOF

# feat-dark-mode: 1 DONE (unread) + 1 WAIT
mkdir -p "$STATUS_DIR/feat-dark-mode"
cat > "$STATUS_DIR/feat-dark-mode/sess-pqr678.json" <<EOF
{"status":"DONE","session_id":"sess-pqr678","timestamp":"$AGO_3M","worktree":"feat-dark-mode"}
EOF
cat > "$STATUS_DIR/feat-dark-mode/sess-stu901.json" <<EOF
{"status":"WAIT","session_id":"sess-stu901","timestamp":"$AGO_1M","worktree":"feat-dark-mode"}
EOF

# fix-api-timeout: 1 DONE
mkdir -p "$STATUS_DIR/fix-api-timeout"
cat > "$STATUS_DIR/fix-api-timeout/sess-vwx234.json" <<EOF
{"status":"DONE","session_id":"sess-vwx234","timestamp":"$AGO_8M","worktree":"fix-api-timeout"}
EOF

echo "Seeded demo data in $STATUS_DIR"
