package tmux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/config"
)

func installFakeTmux(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + logPath + "\n" +
		"case \"$1\" in\n" +
		"  list-sessions) printf 'alpha\\nbeta\\n' ;;\n" +
		"  display-message) printf 'current-session\\n' ;;\n" +
		"  capture-pane) printf 'pane output\\n' ;;\n" +
		"  list-panes) printf '%%1\\n%%2\\n' ;;\n" +
		"  has-session) [ \"$3\" = \"exists\" ] ;;\n" +
		"  *) exit 0 ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(binDir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", binDir)
	return logPath
}

func TestPrepareLayoutCmd_InjectsSessionTarget(t *testing.T) {
	args := prepareLayoutCmd([]string{"split-window", "-h"}, "mysession", "/tmp/dir")

	if args[0] != "split-window" {
		t.Errorf("args[0] = %q, want %q", args[0], "split-window")
	}
	if args[1] != "-t" || args[2] != "mysession" {
		t.Errorf("expected -t mysession, got %v", args[1:3])
	}
}

func TestPrepareLayoutCmd_InjectsWorkingDir(t *testing.T) {
	args := prepareLayoutCmd([]string{"split-window", "-h"}, "sess", "/work")

	found := false
	for i, a := range args {
		if a == "-c" && i+1 < len(args) && args[i+1] == "/work" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected -c /work in args, got %v", args)
	}
}

func TestPrepareLayoutCmd_SkipsTargetIfPresent(t *testing.T) {
	args := prepareLayoutCmd([]string{"split-window", "-t", "custom"}, "sess", "/work")

	count := 0
	for _, a := range args {
		if a == "-t" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 -t flag, got %d in %v", count, args)
	}
}

func TestPrepareLayoutCmd_SkipsDirIfPresent(t *testing.T) {
	args := prepareLayoutCmd([]string{"split-window", "-c", "/custom"}, "sess", "/work")

	count := 0
	for _, a := range args {
		if a == "-c" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 -c flag, got %d in %v", count, args)
	}
}

func TestPrepareLayoutCmd_NoDirForSelectLayout(t *testing.T) {
	args := prepareLayoutCmd([]string{"select-layout", "even-horizontal"}, "sess", "/work")

	for _, a := range args {
		if a == "-c" {
			t.Errorf("select-layout should not get -c flag, got %v", args)
		}
	}
}

func TestPrepareLayoutCmd_NewWindowGetsDir(t *testing.T) {
	args := prepareLayoutCmd([]string{"new-window"}, "sess", "/work")

	found := false
	for i, a := range args {
		if a == "-c" && i+1 < len(args) && args[i+1] == "/work" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("new-window should get -c /work, got %v", args)
	}
}

func TestPrepareLayoutCmd_EmptyArgs(t *testing.T) {
	args := prepareLayoutCmd([]string{}, "sess", "/work")
	if len(args) != 0 {
		t.Errorf("expected empty args, got %v", args)
	}
}

func TestTmuxCommandWrappersUseTmuxCLI(t *testing.T) {
	logPath := installFakeTmux(t)

	t.Setenv("TMUX", "")
	if InTmux() {
		t.Fatal("InTmux() = true with empty TMUX")
	}
	t.Setenv("TMUX", "/tmp/tmux.sock")
	if !InTmux() {
		t.Fatal("InTmux() = false with TMUX set")
	}

	if !SessionExists("exists") {
		t.Fatal("SessionExists(exists) = false")
	}
	if SessionExists("missing") {
		t.Fatal("SessionExists(missing) = true")
	}

	sessions := ListSessions()
	if !sessions["alpha"] || !sessions["beta"] || len(sessions) != 2 {
		t.Fatalf("ListSessions() = %v, want alpha and beta", sessions)
	}
	if err := SendKeys("target", "echo hi", "Enter"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	if err := KillSession("target"); err != nil {
		t.Fatalf("KillSession: %v", err)
	}
	if err := RenameSession("old", "new"); err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	if err := SwitchClient("target"); err != nil {
		t.Fatalf("SwitchClient in tmux: %v", err)
	}
	t.Setenv("TMUX", "")
	if err := SwitchClient("target"); err != nil {
		t.Fatalf("SwitchClient outside tmux: %v", err)
	}
	if got, err := CurrentSession(); err != nil || got != "current-session" {
		t.Fatalf("CurrentSession() = %q, %v; want current-session", got, err)
	}
	if got, err := CapturePane("target"); err != nil || got != "pane output" {
		t.Fatalf("CapturePane() = %q, %v; want pane output", got, err)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read tmux log: %v", err)
	}
	logText := string(logData)
	for _, want := range []string{
		"send-keys -t target echo hi Enter",
		"kill-session -t target",
		"rename-session -t old new",
		"switch-client -t target",
		"attach-session -t target",
		"capture-pane -ept target -S -",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("tmux log missing %q:\n%s", want, logText)
		}
	}
}

func TestNewSessionRunsLayoutAndPaneCommands(t *testing.T) {
	logPath := installFakeTmux(t)

	err := NewSession("sess", "/work", []string{"split-window -h"}, []config.PaneConfig{
		{Command: "echo one"},
		{Command: "echo two"},
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read tmux log: %v", err)
	}
	logText := string(logData)
	for _, want := range []string{
		"new-session -d -s sess -c /work",
		"split-window -t sess -h -c /work",
		"list-panes -t sess -s -F #{pane_id}",
		"send-keys -t %1 echo one Enter",
		"send-keys -t %2 echo two Enter",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("tmux log missing %q:\n%s", want, logText)
		}
	}
}
