package harness

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

var codexHookEvents = []string{
	"UserPromptSubmit",
	"PreToolUse",
	"PermissionRequest",
	"PostToolUse",
	"Stop",
}

func init() {
	Register(Codex{})
}

type Codex struct{}

func (Codex) ID() string                { return CodexID }
func (Codex) DisplayName() string       { return "Codex CLI" }
func (Codex) ExecutableName() string    { return "codex" }
func (Codex) SetupCommandLabel() string { return "ww codex-setup" }
func (Codex) DocsHint() string {
	return "Codex hooks are installed into ~/.codex/hooks.json and may need review with /hooks."
}
func (Codex) HookEvents() []string { return append([]string{}, codexHookEvents...) }
func (Codex) Capabilities() Capabilities {
	return Capabilities{
		SupportsPermissionWait: true,
		SupportsFilesTouched:   true,
		RequiresHookTrust:      true,
		SupportsPermissionMode: true,
		SupportsTurnID:         true,
	}
}

func (Codex) HookCommand(exe string) string {
	return exe + " hook --harness codex"
}

func (h Codex) InstallHooks(command string) (bool, error) {
	before, _ := os.ReadFile(codexHooksPath())
	if err := h.addHookToConfig(command); err != nil {
		return false, err
	}
	after, _ := os.ReadFile(codexHooksPath())
	return !bytes.Equal(before, after), nil
}

func (h Codex) HooksInstalled(command string) bool {
	settings, err := readJSONFile(codexHooksPath())
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

func (Codex) LegacyHooks() []LegacyHook { return nil }

func (Codex) RemoveLegacyHooks() ([]string, bool, error) {
	return nil, false, nil
}

type codexHookInput struct {
	SessionID      string          `json:"session_id"`
	HookEventName  string          `json:"hook_event_name"`
	ToolName       string          `json:"tool_name"`
	FilePath       string          `json:"file_path"`
	Model          string          `json:"model"`
	TurnID         string          `json:"turn_id"`
	PermissionMode string          `json:"permission_mode"`
	ToolInput      json.RawMessage `json:"tool_input"`
}

func (Codex) NormalizeHook(raw []byte) (NormalizedHook, bool) {
	var in codexHookInput
	if err := json.Unmarshal(bytes.TrimSpace(raw), &in); err != nil {
		return NormalizedHook{}, false
	}
	if in.SessionID == "" {
		return NormalizedHook{}, false
	}
	files := extractCodexFiles(in.FilePath, in.ToolInput)
	return NormalizedHook{
		HarnessID:      CodexID,
		SessionID:      in.SessionID,
		EventName:      in.HookEventName,
		ToolName:       in.ToolName,
		FilePath:       in.FilePath,
		FilesTouched:   files,
		Model:          in.Model,
		TurnID:         in.TurnID,
		PermissionMode: in.PermissionMode,
	}, true
}

func (Codex) BuildLaunch(opts LaunchOptions) LaunchCommand {
	return launchCommand("codex", nil, opts, false, []string{"--dangerously-bypass-approvals-and-sandbox"})
}

func (Codex) BuildShellLaunch(opts ShellLaunchOptions) string {
	return shellLaunch("codex", nil, opts, false, []string{"--dangerously-bypass-approvals-and-sandbox"})
}

func (h Codex) addHookToConfig(command string) error {
	settings, err := readJSONFile(codexHooksPath())
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
				"type":          "command",
				"command":       command,
				"statusMessage": "Updating Willow status",
			},
		},
	}

	for _, event := range h.HookEvents() {
		existing, _ := hooksMap[event].([]any)
		filtered := make([]any, 0, len(existing))
		for _, rule := range existing {
			if !isWillowCodexRule(rule) {
				filtered = append(filtered, rule)
			}
		}
		hooksMap[event] = append(filtered, willowRule)
	}

	settings["hooks"] = hooksMap
	return writeJSONFile(codexHooksPath(), settings)
}

func codexHooksPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "hooks.json")
}

func isWillowCodexRule(rule any) bool {
	ruleMap, ok := rule.(map[string]any)
	if !ok {
		return false
	}
	for _, cmd := range ruleCommands(ruleMap) {
		cmd = strings.TrimSpace(cmd)
		if strings.Contains(cmd, " hook --harness codex") && strings.Contains(cmd, "willow") {
			return true
		}
	}
	return false
}

func extractCodexFiles(topLevelFile string, toolInput json.RawMessage) []string {
	seen := map[string]bool{}
	var files []string
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		files = append(files, path)
	}
	add(topLevelFile)

	if len(toolInput) == 0 {
		return files
	}

	var input map[string]any
	if err := json.Unmarshal(toolInput, &input); err != nil {
		var s string
		if err := json.Unmarshal(toolInput, &s); err == nil {
			extractPatchFilesFromText(s, add)
		}
		return files
	}

	for _, key := range []string{"file_path", "path", "target_file", "target_path"} {
		if v, ok := input[key].(string); ok {
			add(v)
		}
	}
	for _, key := range []string{"command", "patch", "input"} {
		if v, ok := input[key].(string); ok {
			extractPatchFilesFromText(v, add)
		}
	}
	return files
}

func extractPatchFilesFromText(text string, add func(string)) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		for _, prefix := range []string{
			"*** Add File:",
			"*** Update File:",
			"*** Delete File:",
		} {
			if path, ok := strings.CutPrefix(line, prefix); ok {
				add(path)
			}
		}
	}
}
