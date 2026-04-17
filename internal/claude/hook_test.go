package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstall_WritesSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := Install(); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	if _, err := os.Stat(StatusDir()); err != nil {
		t.Fatalf("status dir not created: %v", err)
	}

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

	// Every registered command must end in " hook"
	cmd, err := hookCommand()
	if err != nil {
		t.Fatalf("hookCommand error: %v", err)
	}
	if !strings.HasSuffix(cmd, " hook") {
		t.Errorf("hookCommand = %q, want suffix %q", cmd, " hook")
	}
}

// TestHookCommandFor_PreservesSymlinks guards against regressing to
// filepath.EvalSymlinks. On Homebrew, the stable launcher at
// /opt/homebrew/bin/willow is a symlink into a versioned Cellar path that
// vanishes on `brew upgrade`, so hook registration must use the launcher path
// verbatim instead of resolving through it.
func TestHookCommandFor_PreservesSymlinks(t *testing.T) {
	launcher := "/opt/homebrew/bin/willow"
	got := hookCommandFor(launcher)
	if got != launcher+" hook" {
		t.Errorf("hookCommandFor(%q) = %q, want %q", launcher, got, launcher+" hook")
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

	if _, err := Install(); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	if !IsInstalled() {
		t.Error("IsInstalled should be true after Install()")
	}
}

func TestInstall_Idempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	changed1, err := Install()
	if err != nil {
		t.Fatalf("first Install() error: %v", err)
	}
	if !changed1 {
		t.Error("first Install() should report changed=true")
	}
	changed2, err := Install()
	if err != nil {
		t.Fatalf("second Install() error: %v", err)
	}
	if changed2 {
		t.Error("second Install() should report changed=false")
	}

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

// TestInstall_ReplacesOnlyMarkedRules verifies Install() replaces rules
// carrying "source":"willow" (idempotency across upgrades that move the
// willow binary) and leaves everything else alone — legacy willow artifacts
// are flagged by `ww doctor`, not auto-mutated.
func TestInstall_ReplacesOnlyMarkedRules(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(settingsPath), 0o755)
	seed := map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				// Marked willow rule with stale path — MUST be replaced
				map[string]any{
					"source": "willow",
					"hooks": []any{map[string]any{
						"type":    "command",
						"command": "/old/path/to/willow hook",
					}},
				},
				// Legacy unmarked shell hook — MUST be preserved (doctor's job)
				map[string]any{
					"hooks": []any{map[string]any{
						"type":    "command",
						"command": "/Users/x/.willow/hooks/claude-status-hook.sh",
					}},
				},
				// Unrelated third-party rule — MUST be preserved
				map[string]any{
					"hooks": []any{map[string]any{
						"type":    "command",
						"command": "/opt/third-party/notify.sh",
					}},
				},
			},
		},
	}
	data, _ := json.Marshal(seed)
	os.WriteFile(settingsPath, data, 0o644)

	if _, err := Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}

	got, _ := readClaudeSettings()
	rules := got["hooks"].(map[string]any)["Stop"].([]any)

	if len(rules) != 3 {
		t.Fatalf("Stop rule count = %d, want 3 (1 replaced willow + 2 preserved)", len(rules))
	}

	var markedWillow, legacyShell, thirdParty int
	for _, r := range rules {
		rm := r.(map[string]any)
		if src, _ := rm["source"].(string); src == "willow" {
			markedWillow++
			continue
		}
		inner := rm["hooks"].([]any)
		cmd := inner[0].(map[string]any)["command"].(string)
		switch {
		case strings.HasSuffix(cmd, "claude-status-hook.sh"):
			legacyShell++
		case strings.Contains(cmd, "third-party"):
			thirdParty++
		default:
			t.Errorf("unexpected residual rule: %s", cmd)
		}
	}

	if markedWillow != 1 {
		t.Errorf("marked willow rule count = %d, want 1", markedWillow)
	}
	if legacyShell != 1 {
		t.Errorf("legacy shell hook count = %d, want 1 (preserved for doctor)", legacyShell)
	}
	if thirdParty != 1 {
		t.Errorf("third-party rule count = %d, want 1", thirdParty)
	}

	// The stale "/old/path/to/willow hook" must be gone.
	for _, r := range rules {
		rm := r.(map[string]any)
		if inner, ok := rm["hooks"].([]any); ok {
			for _, h := range inner {
				if cmd, _ := h.(map[string]any)["command"].(string); strings.HasPrefix(cmd, "/old/path") {
					t.Errorf("stale marked willow rule should have been replaced: %s", cmd)
				}
			}
		}
	}
}

// TestUnmarkedLegacyHooks verifies doctor's legacy detector recognizes hooks
// from before the "source":"willow" marker existed.
func TestUnmarkedLegacyHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(settingsPath), 0o755)
	seed := map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				// Legacy shell hook
				map[string]any{
					"hooks": []any{map[string]any{
						"type":    "command",
						"command": "/Users/x/.willow/hooks/claude-status-hook.sh",
					}},
				},
				// Legacy binary hook without marker
				map[string]any{
					"hooks": []any{map[string]any{
						"type":    "command",
						"command": "/usr/local/bin/willow hook",
					}},
				},
				// Current marked rule — NOT legacy
				map[string]any{
					"source": "willow",
					"hooks": []any{map[string]any{
						"type":    "command",
						"command": "/opt/homebrew/bin/willow hook",
					}},
				},
				// Third-party rule — NOT legacy
				map[string]any{
					"hooks": []any{map[string]any{
						"type":    "command",
						"command": "/opt/third-party/notify.sh",
					}},
				},
			},
		},
	}
	data, _ := json.Marshal(seed)
	os.WriteFile(settingsPath, data, 0o644)

	legacy := UnmarkedLegacyHooks()
	if len(legacy) != 2 {
		t.Fatalf("legacy count = %d, want 2: %v", len(legacy), legacy)
	}
	found := map[string]bool{}
	for _, h := range legacy {
		found[h.Command] = true
		if h.Event != "Stop" {
			t.Errorf("unexpected event %q for %q", h.Event, h.Command)
		}
	}
	if !found["/Users/x/.willow/hooks/claude-status-hook.sh"] {
		t.Errorf("legacy shell hook not reported: %v", legacy)
	}
	if !found["/usr/local/bin/willow hook"] {
		t.Errorf("legacy binary hook not reported: %v", legacy)
	}
}

// TestRemoveLegacyWillowHooks verifies doctor's --fix path strips unmarked
// legacy rules while leaving marker-tagged willow rules and third-party rules
// in place.
func TestRemoveLegacyWillowHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(settingsPath), 0o755)
	seed := map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				// Legacy shell hook — MUST be removed
				map[string]any{
					"hooks": []any{map[string]any{
						"type":    "command",
						"command": "/Users/x/.willow/hooks/claude-status-hook.sh",
					}},
				},
				// Legacy binary hook without marker — MUST be removed
				map[string]any{
					"hooks": []any{map[string]any{
						"type":    "command",
						"command": "/usr/local/bin/willow hook",
					}},
				},
				// Current marked rule — MUST stay
				map[string]any{
					"source": "willow",
					"hooks": []any{map[string]any{
						"type":    "command",
						"command": "/opt/homebrew/bin/willow hook",
					}},
				},
				// Third-party rule — MUST stay
				map[string]any{
					"hooks": []any{map[string]any{
						"type":    "command",
						"command": "/opt/third-party/notify.sh",
					}},
				},
			},
		},
	}
	data, _ := json.Marshal(seed)
	os.WriteFile(settingsPath, data, 0o644)

	removed, changed, err := RemoveLegacyWillowHooks()
	if err != nil {
		t.Fatalf("RemoveLegacyWillowHooks: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if len(removed) != 2 {
		t.Errorf("removed count = %d, want 2: %v", len(removed), removed)
	}

	got, _ := readClaudeSettings()
	rules := got["hooks"].(map[string]any)["Stop"].([]any)
	if len(rules) != 2 {
		t.Fatalf("remaining rule count = %d, want 2 (marked willow + third-party)", len(rules))
	}
	for _, r := range rules {
		rm := r.(map[string]any)
		inner := rm["hooks"].([]any)
		cmd := inner[0].(map[string]any)["command"].(string)
		if strings.HasSuffix(cmd, "claude-status-hook.sh") || strings.HasPrefix(cmd, "/usr/local/bin") {
			t.Errorf("legacy rule %q was not removed", cmd)
		}
	}

	// Second call is a no-op.
	removed2, changed2, err := RemoveLegacyWillowHooks()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if changed2 || len(removed2) != 0 {
		t.Errorf("second call changed=%v removed=%v, want no-op", changed2, removed2)
	}
}

func TestEventHasHook_FlatFormat(t *testing.T) {
	hooksMap := map[string]any{
		"Stop": []any{
			map[string]any{
				"type":    "command",
				"command": "/abs/willow hook",
			},
		},
	}
	if !eventHasHook(hooksMap, "Stop", "/abs/willow hook") {
		t.Error("should find hook in flat format")
	}
	if eventHasHook(hooksMap, "Stop", "/other/willow hook") {
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
						"command": "/abs/willow hook",
					},
				},
			},
		},
	}
	if !eventHasHook(hooksMap, "Stop", "/abs/willow hook") {
		t.Error("should find hook in nested format")
	}
}

func TestEventHasHook_MissingEvent(t *testing.T) {
	hooksMap := map[string]any{}
	if eventHasHook(hooksMap, "Stop", "/abs/willow hook") {
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

	sessDir := filepath.Join(StatusDir(), repoName, wtName)
	os.MkdirAll(sessDir, 0o755)
	os.WriteFile(filepath.Join(sessDir, "s1.json"), []byte("{}"), 0o644)

	RemoveStatusDir(repoName, wtName)

	if _, err := os.Stat(sessDir); !os.IsNotExist(err) {
		t.Error("session dir should be removed")
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
