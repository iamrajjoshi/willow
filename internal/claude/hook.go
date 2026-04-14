package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func HooksDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".willow", "hooks")
}

func HookScriptPath() string {
	return filepath.Join(HooksDir(), "claude-status-hook.sh")
}

func claudeSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

// hookEvents lists all Claude Code hook events the willow hook should register for.
var hookEvents = []string{
	"UserPromptSubmit",
	"PreToolUse",
	"PostToolUse",
	"Stop",
	"Notification",
	"SessionEnd",
}

// HookScript returns the shell script content for the Claude status hook.
// It reads stdin JSON to extract session_id, hook_event_name, and tool_name,
// and writes per-session status files to ~/.willow/status/<repo>/<worktree>/<session_id>.json.
func HookScript() string {
	return `#!/usr/bin/env bash
# Willow Claude Code status hook
# Writes agent status to ~/.willow/status/<repo>/<worktree>/<session_id>.json

set -euo pipefail

WILLOW_HOME="$HOME/.willow"
STATUS_DIR="$WILLOW_HOME/status"

# Only run inside a willow-managed worktree
REPOS_DIR="$WILLOW_HOME/repos"
WT_DIR="$WILLOW_HOME/worktrees"

resolve_path() {
  cd "$1" 2>/dev/null && pwd -P
}

CWD="$(resolve_path "$PWD")" || exit 0
RESOLVED_WT_DIR="$(resolve_path "$WT_DIR")" || exit 0

# Check if we're under the worktrees directory
case "$CWD" in
  "$RESOLVED_WT_DIR"/*)
    REL="${CWD#"$RESOLVED_WT_DIR"/}"
    ;;
  *)
    exit 0
    ;;
esac

# Extract repo name and worktree dir name
REPO_NAME="${REL%%/*}"
WT_NAME="${REL#*/}"
WT_NAME="${WT_NAME%%/*}"

if [ -z "$REPO_NAME" ] || [ -z "$WT_NAME" ]; then
  exit 0
fi

# Read stdin JSON once
INPUT="$(cat)"

# Parse fields from stdin JSON using sed
SESSION_ID="$(echo "$INPUT" | sed -n 's/.*"session_id" *: *"\([^"]*\)".*/\1/p' | head -1)"
HOOK_EVENT="$(echo "$INPUT" | sed -n 's/.*"hook_event_name" *: *"\([^"]*\)".*/\1/p' | head -1)"
TOOL_NAME="$(echo "$INPUT" | sed -n 's/.*"tool_name" *: *"\([^"]*\)".*/\1/p' | head -1)"

if [ -z "$SESSION_ID" ]; then
  exit 0
fi

DEST_DIR="$STATUS_DIR/$REPO_NAME/$WT_NAME"

# Determine status from the hook event
STATUS="BUSY"
TOOL_FIELD=""

case "$HOOK_EVENT" in
  SessionEnd)
    rm -f "$DEST_DIR/$SESSION_ID.json"
    rm -f "$DEST_DIR/$SESSION_ID.files"
    exit 0
    ;;
  UserPromptSubmit)
    STATUS="BUSY"
    ;;
  PreToolUse)
    STATUS="BUSY"
    if [ -n "$TOOL_NAME" ]; then
      TOOL_FIELD="\"tool\":\"$TOOL_NAME\","
    fi
    ;;
  PostToolUse)
    if [ "$TOOL_NAME" = "AskUserQuestion" ]; then
      STATUS="WAIT"
    else
      STATUS="BUSY"
    fi
    ;;
  Stop)
    STATUS="DONE"
    ;;
  Notification)
    # Don't overwrite DONE or BUSY — only set WAIT if currently idle/unknown
    DEST_FILE="$DEST_DIR/$SESSION_ID.json"
    if [ -f "$DEST_FILE" ]; then
      CURRENT="$(sed -n 's/.*"status" *: *"\([^"]*\)".*/\1/p' "$DEST_FILE" | head -1)"
      if [ "$CURRENT" = "DONE" ] || [ "$CURRENT" = "BUSY" ]; then
        exit 0
      fi
    fi
    STATUS="WAIT"
    ;;
esac

mkdir -p "$DEST_DIR"

# --- Enrichment: read existing session data ---
DEST_FILE="$DEST_DIR/$SESSION_ID.json"
TOOL_COUNT=0
START_TIME=""
if [ -f "$DEST_FILE" ]; then
  TOOL_COUNT="$(sed -n 's/.*"tool_count":\([0-9]*\).*/\1/p' "$DEST_FILE" | head -1)"
  START_TIME="$(sed -n 's/.*"start_time":"\([^"]*\)".*/\1/p' "$DEST_FILE" | head -1)"
fi
TOOL_COUNT="${TOOL_COUNT:-0}"

case "$HOOK_EVENT" in
  PreToolUse)
    TOOL_COUNT=$((TOOL_COUNT + 1))
    ;;
esac

NOW="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
START_TIME="${START_TIME:-$NOW}"

# Track files touched by write tools
case "$HOOK_EVENT" in
  PreToolUse)
    case "$TOOL_NAME" in
      Write|Edit|NotebookEdit)
        FILE_PATH="$(echo "$INPUT" | sed -n 's/.*"file_path" *: *"\([^"]*\)".*/\1/p' | head -1)"
        if [ -n "$FILE_PATH" ]; then
          FLIST="$DEST_DIR/$SESSION_ID.files"
          if [ ! -f "$FLIST" ] || ! grep -qxF "$FILE_PATH" "$FLIST"; then
            echo "$FILE_PATH" >> "$FLIST"
          fi
        fi
        ;;
    esac
    ;;
esac

# Write enriched status file
cat > "$DEST_FILE" <<STATUSEOF
{"status":"$STATUS",${TOOL_FIELD}"session_id":"$SESSION_ID","timestamp":"$NOW","worktree":"$WT_NAME","tool_count":$TOOL_COUNT,"start_time":"$START_TIME"}
STATUSEOF
`
}

// Install creates the hook script and adds it to ~/.claude/settings.json.
func Install() error {
	hookPath := HookScriptPath()
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}
	if err := os.WriteFile(hookPath, []byte(HookScript()), 0o755); err != nil {
		return fmt.Errorf("failed to write hook script: %w", err)
	}

	if err := os.MkdirAll(StatusDir(), 0o755); err != nil {
		return fmt.Errorf("failed to create status directory: %w", err)
	}

	if err := addHookToSettings(hookPath); err != nil {
		return fmt.Errorf("failed to update Claude settings: %w", err)
	}

	return nil
}

// IsInstalled checks if the hook is already configured in ~/.claude/settings.json
// for all required hook events.
func IsInstalled() bool {
	settings, err := readClaudeSettings()
	if err != nil {
		return false
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return false
	}

	hookPath := HookScriptPath()
	for _, event := range hookEvents {
		if !eventHasHook(hooksMap, event, hookPath) {
			return false
		}
	}
	return true
}

func eventHasHook(hooksMap map[string]any, event, hookPath string) bool {
	rules, ok := hooksMap[event].([]any)
	if !ok {
		return false
	}
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]any)
		if !ok {
			continue
		}
		// Check nested format: {"hooks": [{"type": "command", "command": "..."}]}
		if innerHooks, ok := ruleMap["hooks"].([]any); ok {
			for _, h := range innerHooks {
				if hMap, ok := h.(map[string]any); ok {
					if cmd, ok := hMap["command"].(string); ok && cmd == hookPath {
						return true
					}
				}
			}
		}
		// Check flat format: {"type": "command", "command": "..."}
		if cmd, ok := ruleMap["command"].(string); ok && cmd == hookPath {
			return true
		}
	}
	return false
}

func readClaudeSettings() (map[string]any, error) {
	path := claudeSettingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]any), nil
		}
		return nil, err
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func addHookToSettings(hookPath string) error {
	settings, err := readClaudeSettings()
	if err != nil {
		return err
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooksMap = make(map[string]any)
	}

	willowRule := map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": hookPath,
			},
		},
	}

	for _, event := range hookEvents {
		if eventHasHook(hooksMap, event, hookPath) {
			continue
		}

		existing, ok := hooksMap[event].([]any)
		if !ok {
			existing = []any{}
		}
		existing = append(existing, willowRule)
		hooksMap[event] = existing
	}

	settings["hooks"] = hooksMap

	settingsPath := claudeSettingsPath()
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, append(data, '\n'), 0o644)
}
