package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/ui"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Fix the login validation bug", "fix-the-login-validation-bug"},
		{"Add retry logic to the API client", "add-retry-logic-to-the"},
		{"simple", "simple"},
		{"UPPERCASE WORDS HERE", "uppercase-words-here"},
		{"special! chars@ here#", "special-chars-here"},
		{"", ""},
		{"a b c d e f g h", "a-b-c-d-e"},
		{"a-very-long-word-that-exceeds-the-fifty-character-limit-by-quite-a-bit", "a-very-long-word-that-exceeds-the-fifty-character"},
	}
	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncatePrompt(t *testing.T) {
	short := "Fix the bug"
	if got := truncatePrompt(short); got != short {
		t.Errorf("truncatePrompt(%q) = %q, want unchanged", short, got)
	}

	long := "This is a very long prompt that exceeds the eighty character limit and should be truncated with an ellipsis at the end"
	got := truncatePrompt(long)
	if len(got) != 80 {
		t.Errorf("truncatePrompt(long) length = %d, want 80", len(got))
	}
	if got[77:] != "..." {
		t.Errorf("truncatePrompt(long) should end with '...', got %q", got[77:])
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "'simple'"},
		{"two words", "'two words'"},
		{"don't panic", "'don'\\''t panic'"},
		{"", "''"},
	}

	for _, tt := range tests {
		if got := shellQuote(tt.input); got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDispatchCmdRequiresPrompt(t *testing.T) {
	err := runApp("dispatch")
	if err == nil {
		t.Fatal("dispatch without prompt should fail")
	}
	if !strings.Contains(err.Error(), "prompt is required") {
		t.Fatalf("error = %v, want prompt required", err)
	}
}

func TestDispatchForegroundRunsClaudeInWorktree(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "shell.log")
	writeTestExecutable(t, binDir, "claude", "#!/bin/sh\nexit 0\n")
	shellPath := writeTestExecutable(t, binDir, "fake-shell", "#!/bin/sh\n"+
		"printf 'cwd=%s\\n' \"$PWD\" >> "+logPath+"\n"+
		"printf 'args=%s|%s|%s\\n' \"$1\" \"$2\" \"$3\" >> "+logPath+"\n")
	t.Setenv("PATH", binDir)
	t.Setenv("SHELL", shellPath)

	wtPath := t.TempDir()
	var buf bytes.Buffer
	u := &ui.UI{Out: &buf}
	if err := dispatchForeground(u, wtPath, "dispatch--thing", "fix don't panic", true); err != nil {
		t.Fatalf("dispatchForeground: %v", err)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read shell log: %v", err)
	}
	logText := string(logData)
	if !strings.Contains(logText, "cwd="+wtPath) {
		t.Fatalf("shell log missing cwd %q:\n%s", wtPath, logText)
	}
	for _, want := range []string{"-l", "-c", "claude 'fix don'\\''t panic'", "--dangerously-skip-permissions"} {
		if !strings.Contains(logText, want) {
			t.Fatalf("shell log missing %q:\n%s", want, logText)
		}
	}
	if !strings.Contains(buf.String(), "Dispatched agent") {
		t.Fatalf("UI output missing dispatch success:\n%s", buf.String())
	}
}

func TestDispatchForegroundMissingClaude(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := dispatchForeground(&ui.UI{}, t.TempDir(), "branch", "prompt", false)
	if err == nil {
		t.Fatal("dispatchForeground should fail without claude in PATH")
	}
	if !strings.Contains(err.Error(), "'claude' CLI not found") {
		t.Fatalf("error = %v, want missing claude message", err)
	}
}
