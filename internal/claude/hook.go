package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

// hookCommand returns the shell-quoted command string that Claude Code will
// invoke for every registered hook event: `<abs path to willow> hook`.
// The absolute path is resolved once at install time via os.Executable() so
// the registration survives PATH changes.
func hookCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve willow executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe + " hook", nil
}

// Install registers the willow `hook` subcommand for every required Claude
// Code event in ~/.claude/settings.json and makes sure the status directory
// exists. Returns changed=true when settings.json bytes differ before and
// after — a no-op reinstall returns changed=false.
func Install() (changed bool, err error) {
	if err := os.MkdirAll(StatusDir(), 0o755); err != nil {
		return false, fmt.Errorf("failed to create status directory: %w", err)
	}

	cmd, err := hookCommand()
	if err != nil {
		return false, err
	}

	before, _ := os.ReadFile(claudeSettingsPath())
	if err := addHookToSettings(cmd); err != nil {
		return false, fmt.Errorf("failed to update Claude settings: %w", err)
	}
	after, _ := os.ReadFile(claudeSettingsPath())

	return !bytes.Equal(before, after), nil
}

// UnmarkedLegacyHooks returns command strings in ~/.claude/settings.json that
// look like willow-installed hooks from an older release (no "source":"willow"
// marker). Reported by `ww doctor`; removed only via `ww doctor --fix`. If
// settings.json is unreadable or malformed, returns nil — doctor will surface
// that elsewhere.
func UnmarkedLegacyHooks() []string {
	settings, err := readClaudeSettings()
	if err != nil {
		return nil
	}
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil
	}

	var found []string
	seen := map[string]bool{}
	for _, event := range hookEvents {
		rules, _ := hooksMap[event].([]any)
		for _, rule := range rules {
			ruleMap, ok := rule.(map[string]any)
			if !ok {
				continue
			}
			if src, _ := ruleMap["source"].(string); src == "willow" {
				continue
			}
			for _, cmd := range ruleCommands(ruleMap) {
				if looksLikeLegacyWillowCommand(cmd) && !seen[cmd] {
					seen[cmd] = true
					found = append(found, cmd)
				}
			}
		}
	}
	return found
}

func ruleCommands(ruleMap map[string]any) []string {
	var out []string
	if inner, ok := ruleMap["hooks"].([]any); ok {
		for _, h := range inner {
			if hMap, ok := h.(map[string]any); ok {
				if cmd, ok := hMap["command"].(string); ok {
					out = append(out, cmd)
				}
			}
		}
	}
	if cmd, ok := ruleMap["command"].(string); ok {
		out = append(out, cmd)
	}
	return out
}

// RemoveLegacyWillowHooks strips unmarked willow-looking hook rules from
// ~/.claude/settings.json. Only invoked by `ww doctor --fix` after the user
// confirms. Returns the removed command strings so the caller can report
// what changed, and a changed flag so the caller knows whether settings.json
// was rewritten.
func RemoveLegacyWillowHooks() (removed []string, changed bool, err error) {
	settings, err := readClaudeSettings()
	if err != nil {
		return nil, false, err
	}
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil, false, nil
	}

	for _, event := range hookEvents {
		rules, _ := hooksMap[event].([]any)
		if len(rules) == 0 {
			continue
		}
		kept := make([]any, 0, len(rules))
		for _, rule := range rules {
			ruleMap, ok := rule.(map[string]any)
			if !ok {
				kept = append(kept, rule)
				continue
			}
			// Marker-tagged willow rules stay; the installer owns those.
			if src, _ := ruleMap["source"].(string); src == "willow" {
				kept = append(kept, rule)
				continue
			}
			// Drop unmarked rules that match the legacy willow shape.
			isLegacy := false
			for _, cmd := range ruleCommands(ruleMap) {
				if looksLikeLegacyWillowCommand(cmd) {
					removed = append(removed, cmd)
					isLegacy = true
				}
			}
			if !isLegacy {
				kept = append(kept, rule)
			}
		}
		if len(kept) != len(rules) {
			changed = true
			hooksMap[event] = kept
		}
	}

	if !changed {
		return nil, false, nil
	}

	settings["hooks"] = hooksMap
	settingsPath := claudeSettingsPath()
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return nil, false, err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, false, err
	}
	if err := os.WriteFile(settingsPath, append(data, '\n'), 0o644); err != nil {
		return nil, false, err
	}
	return removed, true, nil
}

// looksLikeLegacyWillowCommand recognizes hook commands willow wrote before
// the "source":"willow" marker existed: the old shell script at
// ~/.willow/hooks/claude-status-hook.sh, or a command ending in " hook" whose
// path contains "willow".
func looksLikeLegacyWillowCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if strings.HasSuffix(cmd, "/claude-status-hook.sh") {
		return true
	}
	if !strings.HasSuffix(cmd, " hook") {
		return false
	}
	return strings.Contains(strings.TrimSuffix(cmd, " hook"), "willow")
}

// IsInstalled reports whether every required hook event in
// ~/.claude/settings.json points to the current willow binary.
func IsInstalled() bool {
	cmd, err := hookCommand()
	if err != nil {
		return false
	}

	settings, err := readClaudeSettings()
	if err != nil {
		return false
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return false
	}

	for _, event := range hookEvents {
		if !eventHasHook(hooksMap, event, cmd) {
			return false
		}
	}
	return true
}

func eventHasHook(hooksMap map[string]any, event, command string) bool {
	rules, ok := hooksMap[event].([]any)
	if !ok {
		return false
	}
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]any)
		if !ok {
			continue
		}
		// Nested format: {"hooks": [{"type": "command", "command": "..."}]}
		if innerHooks, ok := ruleMap["hooks"].([]any); ok {
			for _, h := range innerHooks {
				if hMap, ok := h.(map[string]any); ok {
					if cmd, ok := hMap["command"].(string); ok && cmd == command {
						return true
					}
				}
			}
		}
		// Flat format: {"type": "command", "command": "..."}
		if cmd, ok := ruleMap["command"].(string); ok && cmd == command {
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

func addHookToSettings(command string) error {
	settings, err := readClaudeSettings()
	if err != nil {
		return err
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooksMap = make(map[string]any)
	}

	willowRule := map[string]any{
		// "source" marks the rule as willow-owned so future Install() runs
		// can replace it without touching third-party rules, regardless of
		// where the willow binary was installed.
		"source": "willow",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": command,
			},
		},
	}

	for _, event := range hookEvents {
		existing, _ := hooksMap[event].([]any)
		// Strip every prior willow-marked rule, then append the current one.
		// Keeps the config idempotent across willow upgrades that change the
		// binary path (e.g. Homebrew vs. /usr/local/bin). Unmarked legacy
		// rules are left in place — `ww doctor` surfaces them for manual
		// cleanup.
		filtered := make([]any, 0, len(existing))
		for _, rule := range existing {
			if !isMarkedWillowRule(rule) {
				filtered = append(filtered, rule)
			}
		}
		hooksMap[event] = append(filtered, willowRule)
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

// isMarkedWillowRule reports whether a hook rule carries the
// "source":"willow" marker. This is the ONLY criterion the installer uses to
// decide what to replace. Unmarked rules — even ones that look willow-owned —
// are left untouched.
func isMarkedWillowRule(rule any) bool {
	ruleMap, ok := rule.(map[string]any)
	if !ok {
		return false
	}
	src, _ := ruleMap["source"].(string)
	return src == "willow"
}
