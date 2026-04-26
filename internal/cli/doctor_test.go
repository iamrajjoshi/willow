package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
)

type fakeDoctorUI struct {
	successes []string
	warnings  []string
	infos     []string
	confirms  []string
	confirm   bool
}

func (u *fakeDoctorUI) Success(msg string) { u.successes = append(u.successes, msg) }
func (u *fakeDoctorUI) Warn(msg string)    { u.warnings = append(u.warnings, msg) }
func (u *fakeDoctorUI) Info(msg string)    { u.infos = append(u.infos, msg) }
func (u *fakeDoctorUI) Confirm(msg string) bool {
	u.confirms = append(u.confirms, msg)
	return u.confirm
}
func (u *fakeDoctorUI) Red(s string) string { return "red:" + s }

func (u *fakeDoctorUI) hasSuccess(substr string) bool { return containsString(u.successes, substr) }
func (u *fakeDoctorUI) hasWarning(substr string) bool { return containsString(u.warnings, substr) }
func (u *fakeDoctorUI) hasInfo(substr string) bool    { return containsString(u.infos, substr) }

func containsString(values []string, substr string) bool {
	for _, value := range values {
		if strings.Contains(value, substr) {
			return true
		}
	}
	return false
}

func writeTestExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", name, err)
	}
	return path
}

func TestDoctorCmd(t *testing.T) {
	cmd := doctorCmd()
	if cmd.Name != "doctor" {
		t.Errorf("expected command name %q, got %q", "doctor", cmd.Name)
	}
	if cmd.Action == nil {
		t.Error("expected non-nil action")
	}
}

func TestCheckGitVersion(t *testing.T) {
	tests := []struct {
		name       string
		script     string
		wantStatus string
		wantText   string
	}{
		{
			name:       "supported",
			script:     "#!/bin/sh\nprintf 'git version 2.45.0\\n'\n",
			wantStatus: "success",
			wantText:   "git 2.45.0",
		},
		{
			name:       "old",
			script:     "#!/bin/sh\nprintf 'git version 2.29.0\\n'\n",
			wantStatus: "warn",
			wantText:   "recommend >= 2.30",
		},
		{
			name:       "unparseable",
			script:     "#!/bin/sh\nprintf 'not a version\\n'\n",
			wantStatus: "stdout",
			wantText:   "could not be parsed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			writeTestExecutable(t, binDir, "git", tt.script)
			t.Setenv("PATH", binDir)

			u := &fakeDoctorUI{}
			out, _ := captureStdout(t, func() error {
				checkGitVersion(u)
				return nil
			})

			switch tt.wantStatus {
			case "success":
				if !u.hasSuccess(tt.wantText) {
					t.Fatalf("successes = %v, want %q", u.successes, tt.wantText)
				}
			case "warn":
				if !u.hasWarning(tt.wantText) {
					t.Fatalf("warnings = %v, want %q", u.warnings, tt.wantText)
				}
			case "stdout":
				if !strings.Contains(out, tt.wantText) {
					t.Fatalf("stdout = %q, want %q", out, tt.wantText)
				}
			}
		})
	}
}

func TestParseGitVersion(t *testing.T) {
	tests := []struct {
		input               string
		major, minor, patch int
		wantErr             bool
	}{
		{"git version 2.45.0", 2, 45, 0, false},
		{"git version 2.30.1", 2, 30, 1, false},
		{"git version 1.8.5", 1, 8, 5, false},
		{"git version 2.39.3 (Apple Git-146)", 2, 39, 3, false},
		{"git version 2.43.0.windows.1", 2, 43, 0, false},
		{"2.45.0", 2, 45, 0, false},
		{"", 0, 0, 0, true},
		{"git version abc.def.ghi", 0, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			major, minor, patch, err := parseGitVersion(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if major != tt.major || minor != tt.minor || patch != tt.patch {
				t.Errorf("got %d.%d.%d, want %d.%d.%d", major, minor, patch, tt.major, tt.minor, tt.patch)
			}
		})
	}
}

func TestCheckBinary(t *testing.T) {
	binDir := t.TempDir()
	writeTestExecutable(t, binDir, "gh", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", binDir)

	u := &fakeDoctorUI{}
	checkBinary(u, "gh", "gh CLI", "https://cli.github.com")
	checkBinary(u, "tmux", "tmux", "https://github.com/tmux/tmux")

	if !u.hasSuccess("gh CLI installed") {
		t.Fatalf("successes = %v, want gh installed", u.successes)
	}
	if !u.hasWarning("tmux not found") {
		t.Fatalf("warnings = %v, want tmux missing", u.warnings)
	}
}

func TestCheckClaudeHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	u := &fakeDoctorUI{}
	checkClaudeHooks(u, false)
	if !u.hasWarning("hooks not installed") {
		t.Fatalf("warnings = %v, want missing hooks warning", u.warnings)
	}

	if _, err := claude.Install(); err != nil {
		t.Fatalf("install hooks: %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	hooks := settings["hooks"].(map[string]any)
	hooks["Stop"] = append(hooks["Stop"].([]any), map[string]any{
		"command": "/opt/homebrew/bin/willow hook",
	})
	updated, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if err := os.WriteFile(settingsPath, updated, 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	u = &fakeDoctorUI{}
	checkClaudeHooks(u, false)
	if !u.hasSuccess("hooks installed") {
		t.Fatalf("successes = %v, want installed hooks", u.successes)
	}
	if !u.hasWarning("legacy willow hook") {
		t.Fatalf("warnings = %v, want legacy hook warning", u.warnings)
	}
	if !u.hasInfo("doctor --fix") {
		t.Fatalf("infos = %v, want fix hint", u.infos)
	}

	u = &fakeDoctorUI{confirm: true}
	checkClaudeHooks(u, true)
	if len(u.confirms) != 1 {
		t.Fatalf("confirms = %v, want one confirmation", u.confirms)
	}
	if !u.hasSuccess("Removed 1 legacy hook") {
		t.Fatalf("successes = %v, want legacy hook removal", u.successes)
	}
	if got := claude.UnmarkedLegacyHooks(); len(got) != 0 {
		t.Fatalf("legacy hooks after fix = %v, want none", got)
	}
}

func TestCheckWillowDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	u := &fakeDoctorUI{}
	out, _ := captureStdout(t, func() error {
		checkWillowDirs(u)
		return nil
	})
	if !strings.Contains(out, "missing") {
		t.Fatalf("stdout = %q, want missing directory output", out)
	}

	for _, dir := range []string{config.WillowHome(), config.ReposDir(), config.WorktreesDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	u = &fakeDoctorUI{}
	checkWillowDirs(u)
	if len(u.successes) != 3 {
		t.Fatalf("successes = %v, want three directory successes", u.successes)
	}
}

func TestCheckStaleSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	u := &fakeDoctorUI{}
	checkStaleSessions(u)
	if !u.hasSuccess("no stale session") {
		t.Fatalf("successes = %v, want no stale sessions", u.successes)
	}

	statusDir := filepath.Join(claude.StatusDir(), "repo", "worktree")
	if err := os.MkdirAll(statusDir, 0o755); err != nil {
		t.Fatalf("mkdir status dir: %v", err)
	}
	session := claude.SessionStatus{
		Status:    claude.StatusBusy,
		SessionID: "s1",
		Timestamp: time.Now().Add(-31 * time.Minute),
	}
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	if err := os.WriteFile(filepath.Join(statusDir, "s1.json"), data, 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	u = &fakeDoctorUI{}
	checkStaleSessions(u)
	if !u.hasWarning("1 stale session") {
		t.Fatalf("warnings = %v, want stale session warning", u.warnings)
	}
}

func TestCheckConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	u := &fakeDoctorUI{}
	checkConfig(u)
	if !u.hasSuccess("config valid") {
		t.Fatalf("successes = %v, want config valid", u.successes)
	}

	cfgPath := config.GlobalConfigPath()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"tmux":{"panes":[{"command":"echo hi"}]}}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	u = &fakeDoctorUI{}
	checkConfig(u)
	if !u.hasWarning("config:") {
		t.Fatalf("warnings = %v, want config warning", u.warnings)
	}
}
