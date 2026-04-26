package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/ui"
)

func TestDetachedWorktreeDirNameValidation(t *testing.T) {
	if got, err := detachedWorktreeDirName("feature/test"); err != nil || got != "feature-test" {
		t.Fatalf("detachedWorktreeDirName() = %q, %v; want feature-test", got, err)
	}
	for _, name := range []string{"", ".", ".."} {
		if _, err := detachedWorktreeDirName(name); err == nil {
			t.Fatalf("detachedWorktreeDirName(%q) should fail", name)
		}
	}
}

func TestRunHooksRunsCommandsInDirectory(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(t.TempDir(), "stdout.log")
	stdout, err := os.Create(stdoutPath)
	if err != nil {
		t.Fatalf("create stdout: %v", err)
	}
	defer stdout.Close()

	var buf bytes.Buffer
	u := &ui.UI{Out: &buf}
	if err := runHooks([]string{"printf 'hello\\n'", "pwd"}, dir, u, stdout); err != nil {
		t.Fatalf("runHooks: %v", err)
	}
	if err := stdout.Close(); err != nil {
		t.Fatalf("close stdout: %v", err)
	}

	data, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "hello") || !strings.Contains(out, dir) {
		t.Fatalf("hook stdout = %q, want command output and working dir", out)
	}
	if !strings.Contains(buf.String(), "printf") || !strings.Contains(buf.String(), "pwd") {
		t.Fatalf("UI output = %q, want hook commands", buf.String())
	}
}

func TestRunHooksStopsOnFailure(t *testing.T) {
	stdout, err := os.Create(filepath.Join(t.TempDir(), "stdout.log"))
	if err != nil {
		t.Fatalf("create stdout: %v", err)
	}
	defer stdout.Close()

	err = runHooks([]string{"exit 7"}, t.TempDir(), &ui.UI{}, stdout)
	if err == nil {
		t.Fatal("runHooks should return failed hook error")
	}
	if !strings.Contains(err.Error(), "hook failed: exit 7") {
		t.Fatalf("error = %v, want hook failed context", err)
	}
}

func TestRunPostCheckoutHookInvokesConfiguredHook(t *testing.T) {
	repoDir := t.TempDir()
	repoGit := &git.Git{Dir: repoDir}
	if _, err := repoGit.Run("init"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	configureGitUser(t, repoDir)
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if _, err := repoGit.Run("add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := repoGit.Run("commit", "-m", "initial"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	logPath := filepath.Join(t.TempDir(), "hook.log")
	hookDir := filepath.Join(repoDir, ".hooks")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatalf("mkdir hook dir: %v", err)
	}
	hookPath := filepath.Join(hookDir, "post-checkout")
	hookScript := "#!/bin/sh\nprintf '%s|%s|%s\\n' \"$1\" \"$2\" \"$3\" > " + logPath + "\n"
	if err := os.WriteFile(hookPath, []byte(hookScript), 0o755); err != nil {
		t.Fatalf("write hook: %v", err)
	}

	runPostCheckoutHook("", repoDir, &ui.UI{}, false)
	runPostCheckoutHook(".hooks/missing", repoDir, &ui.UI{}, false)
	runPostCheckoutHook(".hooks/post-checkout", repoDir, &ui.UI{}, true)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read hook log: %v", err)
	}
	logText := string(data)
	if !strings.HasPrefix(logText, "0000000000000000000000000000000000000000|") {
		t.Fatalf("hook log = %q, want null old ref", logText)
	}
	if !strings.HasSuffix(strings.TrimSpace(logText), "|1") {
		t.Fatalf("hook log = %q, want checkout flag 1", logText)
	}
}
