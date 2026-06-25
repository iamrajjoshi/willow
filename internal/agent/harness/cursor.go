package harness

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

var cursorHookEvents = []string{
	"sessionStart",
	"beforeSubmitPrompt",
	"preToolUse",
	"postToolUse",
	"postToolUseFailure",
	"beforeShellExecution",
	"afterShellExecution",
	"afterFileEdit",
	"stop",
	"sessionEnd",
}

func init() {
	Register(Cursor{})
}

type Cursor struct{}

func (Cursor) ID() string                { return CursorID }
func (Cursor) DisplayName() string       { return "Cursor Agent" }
func (Cursor) ExecutableName() string    { return "cursor-agent" }
func (Cursor) SetupCommandLabel() string { return "ww cursor-setup" }
func (Cursor) DocsHint() string {
	return "Cursor hooks are installed into ~/.cursor/hooks.json."
}
func (Cursor) HookEvents() []string { return append([]string{}, cursorHookEvents...) }
func (Cursor) Capabilities() Capabilities {
	return Capabilities{
		SupportsSessionEnd:   true,
		SupportsFilesTouched: true,
		SupportsTurnID:       true,
	}
}

func (Cursor) HookCommand(exe string) string {
	return exe + " hook --harness cursor"
}

func (h Cursor) InstallHooks(command string) (bool, error) {
	before, _ := os.ReadFile(cursorHooksPath())
	if err := h.addHookToConfig(command); err != nil {
		return false, err
	}
	after, _ := os.ReadFile(cursorHooksPath())
	return !bytes.Equal(before, after), nil
}

func (h Cursor) HooksInstalled(command string) bool {
	settings, err := readJSONFile(cursorHooksPath())
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

func (Cursor) LegacyHooks() []LegacyHook { return nil }

func (Cursor) RemoveLegacyHooks() ([]string, bool, error) {
	return nil, false, nil
}

type cursorHookInput struct {
	SessionID      string          `json:"session_id"`
	ConversationID string          `json:"conversation_id"`
	GenerationID   string          `json:"generation_id"`
	HookEventName  string          `json:"hook_event_name"`
	ToolName       string          `json:"tool_name"`
	FilePath       string          `json:"file_path"`
	Model          string          `json:"model"`
	ModelID        string          `json:"model_id"`
	ToolInput      json.RawMessage `json:"tool_input"`
}

func (Cursor) NormalizeHook(raw []byte) (NormalizedHook, bool) {
	var in cursorHookInput
	if err := json.Unmarshal(bytes.TrimSpace(raw), &in); err != nil {
		return NormalizedHook{}, false
	}
	sessionID := nonEmptyString(in.SessionID, in.ConversationID)
	if sessionID == "" {
		return NormalizedHook{}, false
	}
	files := extractCodexFiles(in.FilePath, in.ToolInput)
	return NormalizedHook{
		HarnessID:    CursorID,
		SessionID:    sessionID,
		EventName:    in.HookEventName,
		ToolName:     in.ToolName,
		FilePath:     in.FilePath,
		FilesTouched: files,
		Model:        nonEmptyString(in.Model, in.ModelID),
		TurnID:       in.GenerationID,
	}, true
}

func (Cursor) BuildLaunch(opts LaunchOptions) LaunchCommand {
	return launchCommand("cursor-agent", nil, opts, false, []string{"--force"})
}

func (Cursor) BuildShellLaunch(opts ShellLaunchOptions) string {
	return shellLaunch("cursor-agent", nil, opts, false, []string{"--force"})
}

func (h Cursor) addHookToConfig(command string) error {
	settings, err := readJSONFile(cursorHooksPath())
	if err != nil {
		return err
	}
	if _, ok := settings["version"]; !ok {
		settings["version"] = float64(1)
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooksMap = make(map[string]any)
	}

	willowRule := map[string]any{
		"command": command,
		"timeout": float64(5),
	}

	for _, event := range h.HookEvents() {
		existing, _ := hooksMap[event].([]any)
		filtered := make([]any, 0, len(existing))
		for _, rule := range existing {
			if !isWillowCursorRule(rule) {
				filtered = append(filtered, rule)
			}
		}
		hooksMap[event] = append(filtered, willowRule)
	}

	settings["hooks"] = hooksMap
	return writeJSONFile(cursorHooksPath(), settings)
}

func cursorHooksPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cursor", "hooks.json")
}

func isWillowCursorRule(rule any) bool {
	ruleMap, ok := rule.(map[string]any)
	if !ok {
		return false
	}
	for _, cmd := range ruleCommands(ruleMap) {
		cmd = strings.TrimSpace(cmd)
		if strings.Contains(cmd, " hook --harness cursor") && strings.Contains(cmd, "willow") {
			return true
		}
	}
	return false
}

func nonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
