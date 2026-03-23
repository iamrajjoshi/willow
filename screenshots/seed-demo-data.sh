#!/usr/bin/env bash
# Seeds fake multi-session status data for screenshot/GIF recording
set -euo pipefail

STATUS_DIR="$HOME/.willow/status/acme-app"

# Remove old private-data status dir so it doesn't leak into recordings
rm -rf "$HOME/.willow/status/evergreen"

# Clean up any existing test data
rm -rf "$STATUS_DIR/main"
rm -rf "$STATUS_DIR/search-filters"
rm -rf "$STATUS_DIR/onboarding-flow"
rm -rf "$STATUS_DIR/payments-v2"
rm -rf "$STATUS_DIR/dashboard-ui"
rm -rf "$STATUS_DIR/api-tests"

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

# search-filters: 1 DONE (unread) + 1 WAIT (kept for status demo)
mkdir -p "$STATUS_DIR/search-filters"
cat > "$STATUS_DIR/search-filters/sess-ghi789.json" <<EOF
{"status":"DONE","session_id":"sess-ghi789","timestamp":"$AGO_3M","worktree":"search-filters"}
EOF
cat > "$STATUS_DIR/search-filters/sess-jkl012.json" <<EOF
{"status":"WAIT","session_id":"sess-jkl012","timestamp":"$AGO_1M","worktree":"search-filters"}
EOF

# onboarding-flow: 1 DONE (unread) (kept for status demo)
mkdir -p "$STATUS_DIR/onboarding-flow"
cat > "$STATUS_DIR/onboarding-flow/sess-mno345.json" <<EOF
{"status":"DONE","session_id":"sess-mno345","timestamp":"$AGO_8M","worktree":"onboarding-flow"}
EOF

echo "Seeded demo data in $STATUS_DIR"
