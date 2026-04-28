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

func TestGeneratedDetachedDirNamePattern(t *testing.T) {
	for _, name := range []string{"detached-abcdef1", "detached-0123abc-2", "detached-9999999-42"} {
		if !isGeneratedDetachedDirName(name) {
			t.Fatalf("isGeneratedDetachedDirName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"scratch", "detached-abcdef", "detached-abcdef12", "detached-zzzzzzz", "detached-abcdef1-copy"} {
		if isGeneratedDetachedDirName(name) {
			t.Fatalf("isGeneratedDetachedDirName(%q) = true, want false", name)
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

func TestNewDetachedWithoutExplicitRefUsesBaseBranch(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	if err := runApp("clone", origin, "detachedbase"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	worktreeRoot := filepath.Join(home, ".willow", "worktrees", "detachedbase")
	baseBranch := firstWorktreeDir(t, worktreeRoot)
	if err := os.Chdir(filepath.Join(worktreeRoot, baseBranch)); err != nil {
		t.Fatalf("chdir base worktree: %v", err)
	}

	if err := runApp("new", "scratch-from-base", "--detach", "--base", baseBranch, "--no-fetch"); err != nil {
		t.Fatalf("new detached from base failed: %v", err)
	}

	wtPath := filepath.Join(worktreeRoot, "scratch-from-base")
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("detached worktree missing at %s: %v", wtPath, err)
	}
	head, err := (&git.Git{Dir: wtPath}).Run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse detached head: %v", err)
	}
	if head != "HEAD" {
		t.Fatalf("detached worktree branch = %q, want HEAD", head)
	}
}

func TestResolvePRRefUsesGHCLI(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gh.log")
	writeTestExecutable(t, binDir, "gh", "#!/bin/sh\n"+
		"printf '%s\\n' \"$*\" >> "+shellQuote(logPath)+"\n"+
		"printf 'feature-pr\\n'\n")
	t.Setenv("PATH", binDir)

	got, err := resolvePRRef("42", t.TempDir())
	if err != nil {
		t.Fatalf("resolvePRRef: %v", err)
	}
	if got != "feature-pr" {
		t.Fatalf("resolvePRRef() = %q, want feature-pr", got)
	}
	if logText := readTestFile(t, logPath); !strings.Contains(logText, "pr view 42 --json headRefName -q .headRefName") {
		t.Fatalf("gh log missing pr view invocation:\n%s", logText)
	}
}

func TestResolvePRRefReportsGHFailures(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := resolvePRRef("42", t.TempDir()); err == nil || !strings.Contains(err.Error(), "'gh' CLI not found") {
		t.Fatalf("missing gh error = %v", err)
	}

	binDir := t.TempDir()
	writeTestExecutable(t, binDir, "gh", "#!/bin/sh\nprintf 'bad pr\\n' >&2\nexit 1\n")
	t.Setenv("PATH", binDir)
	if _, err := resolvePRRef("42", t.TempDir()); err == nil || !strings.Contains(err.Error(), "bad pr") {
		t.Fatalf("gh failure error = %v", err)
	}
}

func TestPickExistingBranchReportsNoRemoteBranches(t *testing.T) {
	bareDir := filepath.Join(t.TempDir(), "empty.git")
	if _, err := (&git.Git{}).Run("init", "--bare", bareDir); err != nil {
		t.Fatalf("init bare repo: %v", err)
	}
	repoGit := &git.Git{Dir: bareDir}
	if got, err := pickExistingBranchWithQuery(repoGit, "main"); err == nil || got != "" || !strings.Contains(err.Error(), "no remote branches found") {
		t.Fatalf("pickExistingBranchWithQuery = %q, %v; want no-remotes error", got, err)
	}
}

func TestPickExistingBranchWithQueryFiltersCurrentWorktrees(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	seed := filepath.Join(home, "seed-existing")
	if _, err := (&git.Git{}).Run("clone", origin, seed); err != nil {
		t.Fatalf("clone seed: %v", err)
	}
	configureGitUser(t, seed)
	seedGit := &git.Git{Dir: seed}
	if _, err := seedGit.Run("checkout", "-b", "remote-only"); err != nil {
		t.Fatalf("checkout remote branch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seed, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatalf("write remote file: %v", err)
	}
	if _, err := seedGit.Run("add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := seedGit.Run("commit", "-m", "remote branch"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	if _, err := seedGit.Run("push", "origin", "remote-only"); err != nil {
		t.Fatalf("git push remote-only: %v", err)
	}

	if err := runApp("clone", origin, "existingpick"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	bareDir := filepath.Join(home, ".willow", "repos", "existingpick.git")
	repoGit := &git.Git{Dir: bareDir}
	if _, err := repoGit.Run("fetch", "origin", "remote-only:refs/remotes/origin/remote-only"); err != nil {
		t.Fatalf("fetch remote-only: %v", err)
	}
	t.Setenv("FZF_DEFAULT_OPTS", "--filter=remote-only")

	if got, err := pickExistingBranch(repoGit); err != nil || got != "remote-only" {
		t.Fatalf("pickExistingBranch = %q, %v; want remote-only, nil", got, err)
	}
	if got, err := pickExistingBranchWithQuery(repoGit, "remote"); err != nil || got != "remote-only" {
		t.Fatalf("pickExistingBranchWithQuery = %q, %v; want remote-only, nil", got, err)
	}
}
