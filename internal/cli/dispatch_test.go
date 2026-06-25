package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/agent/harness"
	"github.com/iamrajjoshi/willow/internal/config"
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

func TestDispatchCmdCreatesWorktreeAndRunsForegroundClaude(t *testing.T) {
	home := setupTmuxCommandHome(t, "repo")
	helperLog := filepath.Join(t.TempDir(), "willow-helper.log")
	t.Setenv("WILLOW_TEST_HELPER_PROCESS", "willow")
	t.Setenv("WILLOW_TEST_HELPER_WT_ROOT", filepath.Join(home, ".willow", "worktrees"))
	t.Setenv("WILLOW_TEST_HELPER_LOG", helperLog)

	binDir := t.TempDir()
	agentLog := filepath.Join(t.TempDir(), "agent.log")
	writeTestExecutable(t, binDir, "claude", "#!/bin/sh\n"+
		"printf 'cwd=%s\\n' \"$PWD\" >> "+shellQuote(agentLog)+"\n"+
		"printf 'args=%s\\n' \"$*\" >> "+shellQuote(agentLog)+"\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := runApp("dispatch", "Fix dispatch command", "--repo", "repo", "--name", "dispatch-test", "--base", "main", "--no-fetch", "--yolo"); err != nil {
		t.Fatalf("dispatch command failed: %v", err)
	}

	helperText := readTestFile(t, helperLog)
	if !strings.Contains(helperText, "new --cd --repo repo --base main --no-fetch -- dispatch-test") {
		t.Fatalf("helper log missing dispatch new command:\n%s", helperText)
	}
	agentText := readTestFile(t, agentLog)
	for _, want := range []string{
		filepath.Join(home, ".willow", "worktrees", "repo", "dispatch-test"),
		"Fix dispatch command --dangerously-skip-permissions",
	} {
		if !strings.Contains(agentText, want) {
			t.Fatalf("agent log missing %q:\n%s", want, agentText)
		}
	}
}

func TestDispatchCmdUsesConfiguredDefaultAgent(t *testing.T) {
	home := setupTmuxCommandHome(t, "repo")
	writeGlobalConfigFile(t, `{"agent":{"default":"codex"},"telemetry":false}`)
	helperLog := filepath.Join(t.TempDir(), "willow-helper.log")
	t.Setenv("WILLOW_TEST_HELPER_PROCESS", "willow")
	t.Setenv("WILLOW_TEST_HELPER_WT_ROOT", filepath.Join(home, ".willow", "worktrees"))
	t.Setenv("WILLOW_TEST_HELPER_LOG", helperLog)

	binDir := t.TempDir()
	codexLog := filepath.Join(t.TempDir(), "codex.log")
	writeTestExecutable(t, binDir, "codex", "#!/bin/sh\n"+
		"printf 'cwd=%s\\n' \"$PWD\" >> "+shellQuote(codexLog)+"\n"+
		"printf 'args=%s\\n' \"$*\" >> "+shellQuote(codexLog)+"\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := runApp("dispatch", "Use configured agent", "--repo", "repo", "--name", "dispatch-codex"); err != nil {
		t.Fatalf("dispatch command failed: %v", err)
	}

	codexText := readTestFile(t, codexLog)
	if !strings.Contains(codexText, "Use configured agent") {
		t.Fatalf("codex log missing prompt:\n%s", codexText)
	}
	if !strings.Contains(codexText, filepath.Join(home, ".willow", "worktrees", "repo", "dispatch-codex")) {
		t.Fatalf("codex log missing worktree cwd:\n%s", codexText)
	}
}

func TestDispatchCmdAgentOverrideWinsOverConfigDefault(t *testing.T) {
	home := setupTmuxCommandHome(t, "repo")
	writeGlobalConfigFile(t, `{"agent":{"default":"codex"},"telemetry":false}`)
	helperLog := filepath.Join(t.TempDir(), "willow-helper.log")
	t.Setenv("WILLOW_TEST_HELPER_PROCESS", "willow")
	t.Setenv("WILLOW_TEST_HELPER_WT_ROOT", filepath.Join(home, ".willow", "worktrees"))
	t.Setenv("WILLOW_TEST_HELPER_LOG", helperLog)

	binDir := t.TempDir()
	claudeLog := filepath.Join(t.TempDir(), "claude.log")
	codexLog := filepath.Join(t.TempDir(), "codex.log")
	writeTestExecutable(t, binDir, "claude", "#!/bin/sh\nprintf 'args=%s\\n' \"$*\" >> "+shellQuote(claudeLog)+"\n")
	writeTestExecutable(t, binDir, "codex", "#!/bin/sh\nprintf 'args=%s\\n' \"$*\" >> "+shellQuote(codexLog)+"\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := runApp("dispatch", "Override agent", "--repo", "repo", "--name", "dispatch-override", "--agent", "claude"); err != nil {
		t.Fatalf("dispatch command failed: %v", err)
	}

	if got := readTestFile(t, claudeLog); !strings.Contains(got, "Override agent") {
		t.Fatalf("claude log missing prompt:\n%s", got)
	}
	if _, err := os.Stat(codexLog); !os.IsNotExist(err) {
		t.Fatalf("codex should not have been launched")
	}
}

func TestDispatchCmdRunsCursorWithYolo(t *testing.T) {
	home := setupTmuxCommandHome(t, "repo")
	helperLog := filepath.Join(t.TempDir(), "willow-helper.log")
	t.Setenv("WILLOW_TEST_HELPER_PROCESS", "willow")
	t.Setenv("WILLOW_TEST_HELPER_WT_ROOT", filepath.Join(home, ".willow", "worktrees"))
	t.Setenv("WILLOW_TEST_HELPER_LOG", helperLog)

	binDir := t.TempDir()
	cursorLog := filepath.Join(t.TempDir(), "cursor.log")
	writeTestExecutable(t, binDir, "cursor-agent", "#!/bin/sh\n"+
		"printf 'cwd=%s\\n' \"$PWD\" >> "+shellQuote(cursorLog)+"\n"+
		"printf 'args=%s\\n' \"$*\" >> "+shellQuote(cursorLog)+"\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := runApp("dispatch", "Fix auth", "--repo", "repo", "--name", "dispatch-cursor", "--agent", "cursor", "--yolo"); err != nil {
		t.Fatalf("dispatch command failed: %v", err)
	}

	cursorText := readTestFile(t, cursorLog)
	for _, want := range []string{
		filepath.Join(home, ".willow", "worktrees", "repo", "dispatch-cursor"),
		"args=--force Fix auth",
	} {
		if !strings.Contains(cursorText, want) {
			t.Fatalf("cursor log missing %q:\n%s", want, cursorText)
		}
	}
}

func TestDispatchForegroundRunsClaudeInWorktree(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "agent.log")
	writeTestExecutable(t, binDir, "claude", "#!/bin/sh\n"+
		"printf 'cwd=%s\\n' \"$PWD\" >> "+shellQuote(logPath)+"\n"+
		"printf 'args=%s\\n' \"$*\" >> "+shellQuote(logPath)+"\n")
	t.Setenv("PATH", binDir)

	wtPath := t.TempDir()
	var buf bytes.Buffer
	u := &ui.UI{Out: &buf}
	if err := dispatchForeground(u, wtPath, "dispatch--thing", "fix don't panic", harness.Claude{}, config.DefaultConfig(), true); err != nil {
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
	for _, want := range []string{"fix don't panic --dangerously-skip-permissions"} {
		if !strings.Contains(logText, want) {
			t.Fatalf("shell log missing %q:\n%s", want, logText)
		}
	}
	if !strings.Contains(buf.String(), "Dispatched Claude Code") {
		t.Fatalf("UI output missing dispatch success:\n%s", buf.String())
	}
}

func TestDispatchForegroundMissingClaude(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := dispatchForeground(&ui.UI{}, t.TempDir(), "branch", "prompt", harness.Claude{}, config.DefaultConfig(), false)
	if err == nil {
		t.Fatal("dispatchForeground should fail without claude in PATH")
	}
	if !strings.Contains(err.Error(), `"claude" CLI not found`) {
		t.Fatalf("error = %v, want missing claude message", err)
	}
}
