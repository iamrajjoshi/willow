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

// HookScript returns the shell script content for the Claude status hook.
func HookScript() string {
	return `#!/usr/bin/env bash
# Willow Claude Code status hook
# Writes agent status to ~/.willow/status/<repo>/<worktree>.json

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

# Determine status from the hook event
TOOL_NAME="${CLAUDE_TOOL_NAME:-}"
HOOK_EVENT="${CLAUDE_HOOK_EVENT:-}"
STATUS="BUSY"

if [ "$HOOK_EVENT" = "Stop" ]; then
  STATUS="IDLE"
elif [ "$TOOL_NAME" = "AskUserQuestion" ]; then
  STATUS="WAIT"
fi

# Write status file
mkdir -p "$STATUS_DIR/$REPO_NAME"
cat > "$STATUS_DIR/$REPO_NAME/$WT_NAME.json" <<STATUSEOF
{"status":"$STATUS","timestamp":"$(date -u +%Y-%m-%dT%H:%M:%SZ)","worktree":"$WT_NAME"}
STATUSEOF
`
}

// Install creates the hook script and adds it to ~/.claude/settings.json.
func Install() error {
	// Create hook script
	hookPath := HookScriptPath()
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}
	if err := os.WriteFile(hookPath, []byte(HookScript()), 0o755); err != nil {
		return fmt.Errorf("failed to write hook script: %w", err)
	}

	// Create status directory
	if err := os.MkdirAll(StatusDir(), 0o755); err != nil {
		return fmt.Errorf("failed to create status directory: %w", err)
	}

	// Add hook to Claude settings
	if err := addHookToSettings(hookPath); err != nil {
		return fmt.Errorf("failed to update Claude settings: %w", err)
	}

	return nil
}

// IsInstalled checks if the hook is already configured in ~/.claude/settings.json.
func IsInstalled() bool {
	settings, err := readClaudeSettings()
	if err != nil {
		return false
	}

	hooks, ok := settings["hooks"]
	if !ok {
		return false
	}

	hooksMap, ok := hooks.(map[string]any)
	if !ok {
		return false
	}

	for _, events := range []string{"PostToolUse", "Stop"} {
		eventHooks, ok := hooksMap[events]
		if !ok {
			return false
		}
		hooksList, ok := eventHooks.([]any)
		if !ok {
			return false
		}
		found := false
		for _, h := range hooksList {
			hookMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := hookMap["command"].(string); ok && cmd == HookScriptPath() {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
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

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
	}

	willowHook := map[string]any{
		"type":    "command",
		"command": hookPath,
	}

	for _, event := range []string{"PostToolUse", "Stop"} {
		existing, ok := hooks[event].([]any)
		if !ok {
			existing = []any{}
		}

		// Check if already installed
		alreadyInstalled := false
		for _, h := range existing {
			hookMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := hookMap["command"].(string); ok && cmd == hookPath {
				alreadyInstalled = true
				break
			}
		}

		if !alreadyInstalled {
			existing = append(existing, willowHook)
		}
		hooks[event] = existing
	}

	settings["hooks"] = hooks

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
