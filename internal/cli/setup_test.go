package cli

import (
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

	for _, want := range []string{"Installed Claude Code hooks", "hook:", "status:"} {
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
