package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstall_CreatesHookAndSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := Install(); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	// Hook script should exist
	hookPath := HookScriptPath()
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("hook script not created: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Error("hook script should be executable")
	}

	// Status dir should exist
	if _, err := os.Stat(StatusDir()); err != nil {
		t.Fatalf("status dir not created: %v", err)
	}

	// Claude settings should contain hooks
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings not created: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("invalid settings JSON: %v", err)
	}
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("settings should have hooks key")
	}
	for _, event := range hookEvents {
		if _, ok := hooks[event]; !ok {
			t.Errorf("missing hook event %q", event)
		}
	}
}

func TestIsInstalled_FalseWithNoSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if IsInstalled() {
		t.Error("IsInstalled should be false with no settings")
	}
}

func TestIsInstalled_TrueAfterInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := Install(); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	if !IsInstalled() {
		t.Error("IsInstalled should be true after Install()")
	}
}

func TestInstall_Idempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := Install(); err != nil {
		t.Fatalf("first Install() error: %v", err)
	}
	if err := Install(); err != nil {
		t.Fatalf("second Install() error: %v", err)
	}

	// Should still have exactly 1 hook per event
	settings, err := readClaudeSettings()
	if err != nil {
		t.Fatalf("readClaudeSettings error: %v", err)
	}
	hooks := settings["hooks"].(map[string]any)
	for _, event := range hookEvents {
		rules, ok := hooks[event].([]any)
		if !ok {
			t.Fatalf("event %q is not an array", event)
		}
		if len(rules) != 1 {
			t.Errorf("event %q has %d rules, want 1 (idempotent)", event, len(rules))
		}
	}
}

func TestEventHasHook_FlatFormat(t *testing.T) {
	hooksMap := map[string]any{
		"Stop": []any{
			map[string]any{
				"type":    "command",
				"command": "/path/to/hook.sh",
			},
		},
	}
	if !eventHasHook(hooksMap, "Stop", "/path/to/hook.sh") {
		t.Error("should find hook in flat format")
	}
	if eventHasHook(hooksMap, "Stop", "/other/hook.sh") {
		t.Error("should not find non-matching hook")
	}
}

func TestEventHasHook_NestedFormat(t *testing.T) {
	hooksMap := map[string]any{
		"Stop": []any{
			map[string]any{
				"hooks": []any{
					map[string]any{
						"type":    "command",
						"command": "/path/to/hook.sh",
					},
				},
			},
		},
	}
	if !eventHasHook(hooksMap, "Stop", "/path/to/hook.sh") {
		t.Error("should find hook in nested format")
	}
}

func TestEventHasHook_MissingEvent(t *testing.T) {
	hooksMap := map[string]any{}
	if eventHasHook(hooksMap, "Stop", "/path/to/hook.sh") {
		t.Error("should return false for missing event")
	}
}

func TestReadClaudeSettings_EmptyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	settings, err := readClaudeSettings()
	if err != nil {
		t.Fatalf("readClaudeSettings error: %v", err)
	}
	if len(settings) != 0 {
		t.Errorf("expected empty settings, got %v", settings)
	}
}

func TestReadClaudeSettings_InvalidJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(settingsPath), 0o755)
	os.WriteFile(settingsPath, []byte("{invalid"), 0o644)

	_, err := readClaudeSettings()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestRemoveStatusDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "wt1"

	// Create session dir and legacy file
	sessDir := filepath.Join(StatusDir(), repoName, wtName)
	os.MkdirAll(sessDir, 0o755)
	os.WriteFile(filepath.Join(sessDir, "s1.json"), []byte("{}"), 0o644)

	legacyFile := filepath.Join(StatusDir(), repoName, wtName+".json")
	os.WriteFile(legacyFile, []byte("{}"), 0o644)

	RemoveStatusDir(repoName, wtName)

	if _, err := os.Stat(sessDir); !os.IsNotExist(err) {
		t.Error("session dir should be removed")
	}
	if _, err := os.Stat(legacyFile); !os.IsNotExist(err) {
		t.Error("legacy file should be removed")
	}
}

func TestStatusLabel(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusBusy, "BUSY"},
		{StatusDone, "DONE"},
		{StatusWait, "WAIT"},
		{StatusIdle, "IDLE"},
		{StatusOffline, "--"},
	}
	for _, tt := range tests {
		got := StatusLabel(tt.status)
		if got != tt.want {
			t.Errorf("StatusLabel(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}
