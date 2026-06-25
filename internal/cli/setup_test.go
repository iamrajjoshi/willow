package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/config"
)

func TestSetupCmdInstallsHooksWithExistingTelemetryPreference(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeGlobalConfigFile(t, `{"telemetry":false}`)

	out, err := captureStdout(t, func() error {
		return runApp("cc-setup")
	})
	if err != nil {
		t.Fatalf("cc-setup failed: %v", err)
	}

	for _, want := range []string{"Installed Claude Code hooks", "hook:", "status:", "Desktop notifications are enabled by default", "notify.command"} {
		if !strings.Contains(out, want) {
			t.Fatalf("cc-setup output missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); err != nil {
		t.Fatalf("settings.json not written: %v", err)
	}
	cfg, err := config.LoadFile(config.GlobalConfigPath())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Telemetry == nil || *cfg.Telemetry {
		t.Fatalf("telemetry preference = %v, want false", cfg.Telemetry)
	}
}

func TestCodexSetupCmdInstallsHooksWithExistingTelemetryPreference(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeGlobalConfigFile(t, `{"telemetry":false}`)

	out, err := captureStdout(t, func() error {
		return runApp("codex-setup")
	})
	if err != nil {
		t.Fatalf("codex-setup failed: %v", err)
	}

	for _, want := range []string{"Installed Codex CLI hooks", "hook:", "status:", "/hooks"} {
		if !strings.Contains(out, want) {
			t.Fatalf("codex-setup output missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "hooks.json")); err != nil {
		t.Fatalf("hooks.json not written: %v", err)
	}
}

func TestCursorSetupCmdInstallsHooksWithExistingTelemetryPreference(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeGlobalConfigFile(t, `{"telemetry":false}`)

	out, err := captureStdout(t, func() error {
		return runApp("cursor-setup")
	})
	if err != nil {
		t.Fatalf("cursor-setup failed: %v", err)
	}

	for _, want := range []string{"Installed Cursor Agent hooks", "hook:", "status:", "~/.cursor/hooks.json"} {
		if !strings.Contains(out, want) {
			t.Fatalf("cursor-setup output missing %q:\n%s", want, out)
		}
	}
	path := filepath.Join(home, ".cursor", "hooks.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("hooks.json not written: %v", err)
	}
	settings := readSetupJSON(t, path)
	if settings["version"] != float64(1) {
		t.Fatalf("version = %#v, want 1", settings["version"])
	}
	hooks := settings["hooks"].(map[string]any)
	if _, ok := hooks["sessionStart"]; !ok {
		t.Fatalf("sessionStart hook missing: %#v", hooks)
	}
}

func TestAgentSetupAllIncludesCursor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeGlobalConfigFile(t, `{"telemetry":false}`)

	out, err := captureStdout(t, func() error {
		return runApp("agent", "setup", "all")
	})
	if err != nil {
		t.Fatalf("agent setup all failed: %v", err)
	}
	for _, want := range []string{"Claude Code hooks", "Codex CLI hooks", "Cursor Agent hooks"} {
		if !strings.Contains(out, want) {
			t.Fatalf("agent setup all output missing %q:\n%s", want, out)
		}
	}
	for _, path := range []string{
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(home, ".codex", "hooks.json"),
		filepath.Join(home, ".cursor", "hooks.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s not written: %v", path, err)
		}
	}
}

func TestSetupTargetsAcceptCursor(t *testing.T) {
	ids, err := setupTargets("cursor")
	if err != nil {
		t.Fatalf("setupTargets(cursor): %v", err)
	}
	if len(ids) != 1 || ids[0] != "cursor" {
		t.Fatalf("ids = %v, want [cursor]", ids)
	}

	ids, err = setupTargets("all")
	if err != nil {
		t.Fatalf("setupTargets(all): %v", err)
	}
	if !containsString(ids, "cursor") {
		t.Fatalf("ids = %v, want cursor included", ids)
	}
}

func TestSetupCmdPromptsForTelemetryWhenUnset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdin: %v", err)
	}
	if _, err := w.WriteString("y\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	out, err := captureStdout(t, func() error {
		return runApp("cc-setup")
	})
	if err != nil {
		t.Fatalf("cc-setup failed: %v", err)
	}
	if !strings.Contains(out, "Enable anonymous error telemetry") {
		t.Fatalf("cc-setup output missing telemetry prompt:\n%s", out)
	}
	if !strings.Contains(out, "Telemetry enabled") {
		t.Fatalf("cc-setup output missing telemetry enabled:\n%s", out)
	}

	cfg, err := config.LoadFile(config.GlobalConfigPath())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Telemetry == nil || !*cfg.Telemetry {
		t.Fatalf("telemetry preference = %v, want true", cfg.Telemetry)
	}
}

func readSetupJSON(t *testing.T, path string) map[string]any {
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
