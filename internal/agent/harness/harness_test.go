package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/config"
)

func TestRegistryBuiltIns(t *testing.T) {
	for _, id := range []string{ClaudeID, CodexID, CursorID} {
		h, ok := Get(id)
		if !ok {
			t.Fatalf("missing harness %q", id)
		}
		if h.ID() != id {
			t.Fatalf("harness ID = %q, want %q", h.ID(), id)
		}
	}
}

func TestClaudeHookInstaller(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	h := Claude{}
	command := "/usr/local/bin/willow hook --harness claude"

	changed, err := h.InstallHooks(command)
	if err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}
	if !changed {
		t.Fatal("first install should change settings")
	}
	if !h.HooksInstalled(command) {
		t.Fatal("HooksInstalled returned false")
	}
	changed, err = h.InstallHooks(command)
	if err != nil {
		t.Fatalf("second InstallHooks: %v", err)
	}
	if changed {
		t.Fatal("second install should be idempotent")
	}

	settings := readTestJSON(t, filepath.Join(home, ".claude", "settings.json"))
	hooks := settings["hooks"].(map[string]any)
	for _, event := range h.HookEvents() {
		if _, ok := hooks[event]; !ok {
			t.Fatalf("missing Claude hook event %s", event)
		}
	}
}

func TestCodexHookInstaller(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	h := Codex{}
	command := "/usr/local/bin/willow hook --harness codex"

	changed, err := h.InstallHooks(command)
	if err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}
	if !changed {
		t.Fatal("first install should change hooks.json")
	}
	if !h.HooksInstalled(command) {
		t.Fatal("HooksInstalled returned false")
	}

	settings := readTestJSON(t, filepath.Join(home, ".codex", "hooks.json"))
	hooks := settings["hooks"].(map[string]any)
	for _, event := range h.HookEvents() {
		rules, ok := hooks[event].([]any)
		if !ok || len(rules) != 1 {
			t.Fatalf("event %s rules = %#v, want one rule", event, hooks[event])
		}
	}
}

func TestCursorHookInstaller(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	h := Cursor{}
	command := "/usr/local/bin/willow hook --harness cursor"

	settingsPath := filepath.Join(home, ".cursor", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{
		"hooks": {
			"stop": [
				{"command": "/tmp/other-hook"},
				{"command": "/old/willow hook --harness cursor", "timeout": 1}
			]
		}
	}`), 0o644); err != nil {
		t.Fatalf("write existing settings: %v", err)
	}

	changed, err := h.InstallHooks(command)
	if err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}
	if !changed {
		t.Fatal("first install should change hooks.json")
	}
	if !h.HooksInstalled(command) {
		t.Fatal("HooksInstalled returned false")
	}
	changed, err = h.InstallHooks(command)
	if err != nil {
		t.Fatalf("second InstallHooks: %v", err)
	}
	if changed {
		t.Fatal("second install should be idempotent")
	}

	settings := readTestJSON(t, settingsPath)
	if got := settings["version"]; got == nil {
		t.Fatal("version should be set")
	}
	hooks := settings["hooks"].(map[string]any)
	for _, event := range h.HookEvents() {
		rules, ok := hooks[event].([]any)
		if !ok || len(rules) == 0 {
			t.Fatalf("event %s rules = %#v, want rules", event, hooks[event])
		}
	}
	stopRules := hooks["stop"].([]any)
	if len(stopRules) != 2 {
		t.Fatalf("stop rules = %#v, want preserved third-party hook plus willow hook", stopRules)
	}
	if !eventHasHook(hooks, "stop", "/tmp/other-hook") {
		t.Fatal("third-party stop hook should be preserved")
	}
	if eventHasHook(hooks, "stop", "/old/willow hook --harness cursor") {
		t.Fatal("stale willow cursor hook should be replaced")
	}
}

func TestLaunchBuilders(t *testing.T) {
	claude := Claude{}.BuildLaunch(LaunchOptions{Prompt: "fix bug", Yolo: true})
	if claude.Command != "claude" || strings.Join(claude.Args, " ") != "fix bug --dangerously-skip-permissions" {
		t.Fatalf("Claude launch = %#v", claude)
	}

	codex := Codex{}.BuildLaunch(LaunchOptions{Prompt: "fix bug", Yolo: true})
	if codex.Command != "codex" || strings.Join(codex.Args, " ") != "--dangerously-bypass-approvals-and-sandbox fix bug" {
		t.Fatalf("Codex launch = %#v", codex)
	}

	override := config.AgentHarnessConfig{Command: "my-codex", Args: []string{"--profile", "work"}, YoloArgs: []string{"--all-clear"}}
	custom := Codex{}.BuildLaunch(LaunchOptions{Prompt: "fix bug", Yolo: true, Overrides: override})
	if custom.Command != "my-codex" || strings.Join(custom.Args, " ") != "--profile work --all-clear fix bug" {
		t.Fatalf("custom Codex launch = %#v", custom)
	}

	cursor := Cursor{}.BuildLaunch(LaunchOptions{Prompt: "fix bug", Yolo: true})
	if cursor.Command != "cursor-agent" || strings.Join(cursor.Args, " ") != "--force fix bug" {
		t.Fatalf("Cursor launch = %#v", cursor)
	}
}

func TestCodexNormalizeHook(t *testing.T) {
	raw := []byte(`{
		"session_id": "sess",
		"hook_event_name": "PostToolUse",
		"tool_name": "apply_patch",
		"model": "gpt-5.5",
		"turn_id": "turn",
		"permission_mode": "on-request",
		"tool_input": {"command": "*** Begin Patch\n*** Update File: internal/foo.go\n*** Delete File: old.go\n*** End Patch"}
	}`)
	got, ok := Codex{}.NormalizeHook(raw)
	if !ok {
		t.Fatal("NormalizeHook returned false")
	}
	if got.HarnessID != CodexID || got.SessionID != "sess" || got.EventName != "PostToolUse" {
		t.Fatalf("normalized hook = %#v", got)
	}
	if got.Model != "gpt-5.5" || got.TurnID != "turn" || got.PermissionMode != "on-request" {
		t.Fatalf("metadata not preserved: %#v", got)
	}
	if strings.Join(got.FilesTouched, ",") != "internal/foo.go,old.go" {
		t.Fatalf("FilesTouched = %#v", got.FilesTouched)
	}
}

func TestCursorNormalizeHook(t *testing.T) {
	raw := []byte(`{
		"conversation_id": "conv",
		"generation_id": "gen",
		"hook_event_name": "stop",
		"model_id": "cursor-model"
	}`)
	got, ok := Cursor{}.NormalizeHook(raw)
	if !ok {
		t.Fatal("NormalizeHook returned false")
	}
	if got.HarnessID != CursorID || got.SessionID != "conv" || got.EventName != "stop" {
		t.Fatalf("normalized hook = %#v", got)
	}
	if got.TurnID != "gen" || got.Model != "cursor-model" {
		t.Fatalf("metadata not preserved: %#v", got)
	}
}

func TestCursorNormalizeHookSessionAndFiles(t *testing.T) {
	raw := []byte(`{
		"session_id": "sess",
		"conversation_id": "conv",
		"hook_event_name": "afterFileEdit",
		"file_path": "/tmp/a.go",
		"tool_input": {
			"target_path": "/tmp/b.go",
			"metadata": {"source_path": "/tmp/c.go"},
			"paths": ["/tmp/d.go"]
		}
	}`)
	got, ok := Cursor{}.NormalizeHook(raw)
	if !ok {
		t.Fatal("NormalizeHook returned false")
	}
	if got.SessionID != "sess" {
		t.Fatalf("SessionID = %q, want sess", got.SessionID)
	}
	if strings.Join(got.FilesTouched, ",") != "/tmp/a.go,/tmp/b.go,/tmp/c.go,/tmp/d.go" {
		t.Fatalf("FilesTouched = %#v", got.FilesTouched)
	}
}

func TestCursorNormalizeHookRejectsMissingID(t *testing.T) {
	if _, ok := (Cursor{}).NormalizeHook([]byte(`{"hook_event_name":"stop"}`)); ok {
		t.Fatal("NormalizeHook should reject missing session/conversation id")
	}
	if _, ok := (Cursor{}).NormalizeHook([]byte(`not json`)); ok {
		t.Fatal("NormalizeHook should reject invalid JSON")
	}
}

func readTestJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return out
}
