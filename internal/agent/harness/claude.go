package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var claudeHookEvents = []string{
	"UserPromptSubmit",
	"PreToolUse",
	"PostToolUse",
	"Stop",
	"Notification",
	"SessionEnd",
}

func init() {
	Register(Claude{})
}

type Claude struct{}

func (Claude) ID() string                { return ClaudeID }
func (Claude) DisplayName() string       { return "Claude Code" }
func (Claude) ExecutableName() string    { return "claude" }
func (Claude) SetupCommandLabel() string { return "ww cc-setup" }
func (Claude) DocsHint() string {
	return "Claude Code hooks are installed into ~/.claude/settings.json."
}
func (Claude) HookEvents() []string { return append([]string{}, claudeHookEvents...) }
func (Claude) Capabilities() Capabilities {
	return Capabilities{
		SupportsSessionEnd:     true,
		SupportsFilesTouched:   true,
		SupportsNotification:   true,
		SupportsPermissionWait: true,
	}
}

func (Claude) HookCommand(exe string) string {
	return exe + " hook --harness claude"
}

func (h Claude) InstallHooks(command string) (bool, error) {
	before, _ := os.ReadFile(claudeSettingsPath())
	if err := h.addHookToSettings(command); err != nil {
		return false, err
	}
	after, _ := os.ReadFile(claudeSettingsPath())
	return !bytes.Equal(before, after), nil
}

func (h Claude) HooksInstalled(command string) bool {
	settings, err := readJSONFile(claudeSettingsPath())
	if err != nil {
		return false
	}
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return false
	}
	for _, event := range h.HookEvents() {
		if !eventHasHook(hooksMap, event, command) {
			return false
		}
	}
	return true
}

func (h Claude) LegacyHooks() []LegacyHook {
	settings, err := readJSONFile(claudeSettingsPath())
	if err != nil {
		return nil
	}
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil
	}

	var found []LegacyHook
	for _, event := range h.HookEvents() {
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
				if looksLikeLegacyWillowCommand(cmd) {
					found = append(found, LegacyHook{Event: event, Command: cmd})
				}
			}
		}
	}
	return found
}

func (h Claude) RemoveLegacyHooks() ([]string, bool, error) {
	settings, err := readJSONFile(claudeSettingsPath())
	if err != nil {
		return nil, false, err
	}
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil, false, nil
	}

	var removed []string
	changed := false
	for _, event := range h.HookEvents() {
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
			if src, _ := ruleMap["source"].(string); src == "willow" {
				kept = append(kept, rule)
				continue
			}
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
	if err := writeJSONFile(claudeSettingsPath(), settings); err != nil {
		return nil, false, err
	}
	return removed, true, nil
}

type claudeHookInput struct {
	SessionID     string `json:"session_id"`
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	FilePath      string `json:"file_path"`
}

func (Claude) NormalizeHook(raw []byte) (NormalizedHook, bool) {
	var in claudeHookInput
	if err := json.Unmarshal(bytes.TrimSpace(raw), &in); err != nil {
		return NormalizedHook{}, false
	}
	if in.SessionID == "" {
		return NormalizedHook{}, false
	}
	files := []string{}
	if in.HookEventName == "PreToolUse" && isClaudeWriteTool(in.ToolName) && in.FilePath != "" {
		files = append(files, in.FilePath)
	}
	return NormalizedHook{
		HarnessID:    ClaudeID,
		SessionID:    in.SessionID,
		EventName:    in.HookEventName,
		ToolName:     in.ToolName,
		FilePath:     in.FilePath,
		FilesTouched: files,
	}, true
}

func (Claude) BuildLaunch(opts LaunchOptions) LaunchCommand {
	return launchCommand("claude", nil, opts, true, []string{"--dangerously-skip-permissions"})
}

func (Claude) BuildShellLaunch(opts ShellLaunchOptions) string {
	return shellLaunch("claude", nil, opts, true, []string{"--dangerously-skip-permissions"})
}

func (h Claude) addHookToSettings(command string) error {
	settings, err := readJSONFile(claudeSettingsPath())
	if err != nil {
		return err
	}
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooksMap = make(map[string]any)
	}

	willowRule := map[string]any{
		"source": "willow",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": command,
			},
		},
	}

	for _, event := range h.HookEvents() {
		existing, _ := hooksMap[event].([]any)
		filtered := make([]any, 0, len(existing))
		for _, rule := range existing {
			if !isMarkedWillowRule(rule) {
				filtered = append(filtered, rule)
			}
		}
		hooksMap[event] = append(filtered, willowRule)
	}

	settings["hooks"] = hooksMap
	return writeJSONFile(claudeSettingsPath(), settings)
}

func claudeSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

func readJSONFile(path string) (map[string]any, error) {
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

func writeJSONFile(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
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
		for _, cmd := range ruleCommands(ruleMap) {
			if cmd == command {
				return true
			}
		}
	}
	return false
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

func isMarkedWillowRule(rule any) bool {
	ruleMap, ok := rule.(map[string]any)
	if !ok {
		return false
	}
	src, _ := ruleMap["source"].(string)
	return src == "willow"
}

func looksLikeLegacyWillowCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if strings.HasSuffix(cmd, "/claude-status-hook.sh") {
		return true
	}
	if !strings.HasSuffix(cmd, " hook") && !strings.HasSuffix(cmd, " hook --harness claude") {
		return false
	}
	cmd = strings.TrimSuffix(strings.TrimSuffix(cmd, " hook --harness claude"), " hook")
	return strings.Contains(cmd, "willow")
}

func isClaudeWriteTool(name string) bool {
	return name == "Write" || name == "Edit" || name == "NotebookEdit"
}
