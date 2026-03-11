#!/usr/bin/env bash
# Seeds fake multi-session status data for screenshot/GIF recording
set -euo pipefail

STATUS_DIR="$HOME/.willow/status/evergreen"

# Clean up any existing test data
rm -rf "$STATUS_DIR/master"
rm -rf "$STATUS_DIR/raj--dataset-list-active-header"
rm -rf "$STATUS_DIR/raj--dev-restart-command"

NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
AGO_1M=$(date -u -v-1M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '1 minute ago' +%Y-%m-%dT%H:%M:%SZ)
AGO_3M=$(date -u -v-3M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '3 minutes ago' +%Y-%m-%dT%H:%M:%SZ)
AGO_8M=$(date -u -v-8M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '8 minutes ago' +%Y-%m-%dT%H:%M:%SZ)

# master: 2 BUSY sessions (multi-agent)
mkdir -p "$STATUS_DIR/master"
cat > "$STATUS_DIR/master/sess-abc123.json" <<EOF
{"status":"BUSY","tool":"Edit","session_id":"sess-abc123","timestamp":"$NOW","worktree":"master"}
EOF
cat > "$STATUS_DIR/master/sess-def456.json" <<EOF
{"status":"BUSY","tool":"Bash","session_id":"sess-def456","timestamp":"$AGO_1M","worktree":"master"}
EOF

# raj--dataset-list-active-header: 1 DONE (unread) + 1 WAIT
mkdir -p "$STATUS_DIR/raj--dataset-list-active-header"
cat > "$STATUS_DIR/raj--dataset-list-active-header/sess-ghi789.json" <<EOF
{"status":"DONE","session_id":"sess-ghi789","timestamp":"$AGO_3M","worktree":"raj--dataset-list-active-header"}
EOF
cat > "$STATUS_DIR/raj--dataset-list-active-header/sess-jkl012.json" <<EOF
{"status":"WAIT","session_id":"sess-jkl012","timestamp":"$AGO_1M","worktree":"raj--dataset-list-active-header"}
EOF

# raj--dev-restart-command: 1 DONE (unread)
mkdir -p "$STATUS_DIR/raj--dev-restart-command"
cat > "$STATUS_DIR/raj--dev-restart-command/sess-mno345.json" <<EOF
{"status":"DONE","session_id":"sess-mno345","timestamp":"$AGO_8M","worktree":"raj--dev-restart-command"}
EOF

echo "Seeded demo data in $STATUS_DIR"
