package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/tmux"
	"github.com/iamrajjoshi/willow/internal/worktree"
)

// setupTestEnv creates a fake HOME with an "origin" git repo to clone from.
// Returns the origin repo path. HOME and working directory are automatically
// restored after the test.
func setupTestEnv(t *testing.T) string {
	t.Helper()

	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })

	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a local "origin" repo with an initial commit
	origin := filepath.Join(home, "origin.git")
	g := &git.Git{}

	if _, err := g.Run("init", "--bare", origin); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}

	// We need a working copy to make the initial commit
	workdir := filepath.Join(home, "workdir")
	if _, err := g.Run("clone", origin, workdir); err != nil {
		t.Fatalf("git clone: %v", err)
	}

	wg := &git.Git{Dir: workdir}
	if _, err := wg.Run("config", "user.email", "test@test.com"); err != nil {
		t.Fatalf("git config email: %v", err)
	}
	if _, err := wg.Run("config", "user.name", "Test"); err != nil {
		t.Fatalf("git config name: %v", err)
	}

	readme := filepath.Join(workdir, "README.md")
	os.WriteFile(readme, []byte("# test repo\n"), 0o644)
	if _, err := wg.Run("add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := wg.Run("commit", "-m", "initial commit"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	if _, err := wg.Run("push", "origin", "main"); err != nil {
		// Some git versions default to master
		if _, err := wg.Run("push", "origin", "master"); err != nil {
			t.Fatalf("git push: %v", err)
		}
	}

	return origin
}

func runApp(args ...string) error {
	app := NewApp()
	return app.Run(context.Background(), append([]string{"willow"}, args...))
}

func writeGlobalConfigFile(t *testing.T, contents string) {
	t.Helper()

	path := config.GlobalConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir global config dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
}

func writeActiveSessionFile(t *testing.T, repo, wt, sessionID string, status claude.Status) {
	t.Helper()

	dir := filepath.Join(claude.StatusDir(), repo, wt)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	data, err := json.Marshal(claude.SessionStatus{
		Status:    status,
		SessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("marshal session status: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, sessionID+".json"), data, 0o644); err != nil {
		t.Fatalf("write session file: %v", err)
	}
}

func configureGitUser(t *testing.T, dir string) {
	t.Helper()

	g := &git.Git{Dir: dir}
	if _, err := g.Run("config", "user.email", "test@test.com"); err != nil {
		t.Fatalf("git config email: %v", err)
	}
	if _, err := g.Run("config", "user.name", "Test"); err != nil {
		t.Fatalf("git config name: %v", err)
	}
}

func commitFile(t *testing.T, dir, name, contents, message string) {
	t.Helper()

	configureGitUser(t, dir)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}

	g := &git.Git{Dir: dir}
	if _, err := g.Run("add", name); err != nil {
		t.Fatalf("git add %s: %v", name, err)
	}
	if _, err := g.Run("commit", "-m", message); err != nil {
		t.Fatalf("git commit %s: %v", message, err)
	}
}

func installTestCLIPath(t *testing.T, ghScript string) (string, string) {
	t.Helper()

	binDir := t.TempDir()
	for _, binary := range []string{"git", "cat"} {
		path, err := exec.LookPath(binary)
		if err != nil {
			t.Fatalf("find %s: %v", binary, err)
		}
		if err := os.Symlink(path, filepath.Join(binDir, binary)); err != nil {
			t.Fatalf("symlink %s: %v", binary, err)
		}
	}

	logPath := filepath.Join(binDir, "gh.log")
	if ghScript != "" {
		ghPath := filepath.Join(binDir, "gh")
		if err := os.WriteFile(ghPath, []byte(ghScript), 0o755); err != nil {
			t.Fatalf("write gh stub: %v", err)
		}
	}

	t.Setenv("PATH", binDir)
	return binDir, logPath
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}

	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	runErr := fn()
	_ = w.Close()
	os.Stdout = origStdout

	out, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}
	return string(out), runErr
}

func installMergedStatusGH(t *testing.T, baseBranch, branch, headOID string) string {
	t.Helper()

	script := fmt.Sprintf(`#!/bin/sh
set -eu

if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  shift 2
  search=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --search)
        search="$2"
        shift 2
        ;;
      --state|--json|--limit|-q|--head)
        shift 2
        ;;
      *)
        shift
        ;;
    esac
  done

  case "$search" in
    *"head:%s"*)
      printf '[{"number":42,"title":"Merged PR","headRefName":"%s","headRefOid":"%s","baseRefName":"%s","state":"MERGED","mergedAt":"2026-04-21T18:08:47Z","reviewDecision":"","mergeable":"MERGEABLE","additions":0,"deletions":0,"url":"https://github.com/test/repo/pull/42","statusCheckRollup":[]}]\n'
      ;;
    *)
      printf '[]\n'
      ;;
  esac
  exit 0
fi

printf 'unsupported gh invocation: %%s\n' "$*" >&2
exit 1
`, branch, branch, headOID, baseBranch)

	_, logPath := installTestCLIPath(t, script)
	return logPath
}

func TestClone_CreatesDirectoryStructure(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	err := runApp("clone", origin, "testrepo")
	if err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	bareDir := filepath.Join(home, ".willow", "repos", "testrepo.git")
	if _, err := os.Stat(bareDir); os.IsNotExist(err) {
		t.Errorf("bare repo not created at %s", bareDir)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "testrepo")
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		t.Fatalf("failed to read worktrees dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 worktree, got %d", len(entries))
	}

	// Verify the worktree has a checkout (README.md should exist)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	if _, err := os.Stat(filepath.Join(mainDir, "README.md")); os.IsNotExist(err) {
		t.Error("README.md not found in worktree checkout")
	}
}

func TestClone_DuplicateFails(t *testing.T) {
	origin := setupTestEnv(t)

	if err := runApp("clone", origin, "testrepo"); err != nil {
		t.Fatalf("first clone failed: %v", err)
	}

	err := runApp("clone", origin, "testrepo")
	if err == nil {
		t.Fatal("expected error cloning duplicate repo")
	}
}

func TestNew_CreatesWorktree(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "testrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	// Run new from inside the cloned worktree
	worktreeDir := filepath.Join(home, ".willow", "worktrees", "testrepo")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	os.Chdir(mainDir)

	if err := runApp("new", "test-branch", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	newWtDir := filepath.Join(worktreeDir, "test-branch")
	if _, err := os.Stat(newWtDir); os.IsNotExist(err) {
		t.Errorf("new worktree not created at %s", newWtDir)
	}

	// Verify the branch was created
	bareDir := filepath.Join(home, ".willow", "repos", "testrepo.git")
	repoGit := &git.Git{Dir: bareDir}
	out, err := repoGit.Run("branch", "--list", "test-branch")
	if err != nil {
		t.Fatalf("git branch --list: %v", err)
	}
	if out == "" {
		t.Error("branch 'test-branch' not found")
	}
}

func TestNew_BranchPrefixFromConfig(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "testrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "testrepo")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	os.Chdir(mainDir)

	// Set branchPrefix in local config
	bareDir := filepath.Join(home, ".willow", "repos", "testrepo.git")
	configPath := filepath.Join(bareDir, "willow.json")
	os.WriteFile(configPath, []byte(`{"branchPrefix": "alice"}`), 0o644)

	if err := runApp("new", "my-feature", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	// Branch should be alice/my-feature
	repoGit := &git.Git{Dir: bareDir}
	out, err := repoGit.Run("branch", "--list", "alice/my-feature")
	if err != nil {
		t.Fatalf("git branch --list: %v", err)
	}
	if out == "" {
		t.Error("branch 'alice/my-feature' not found (branchPrefix not applied)")
	}

	// Directory should use dashes: alice-my-feature (not alicemy-feature)
	expectedDir := filepath.Join(worktreeDir, "alice-my-feature")
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Errorf("worktree directory not created at %s (slashes should become dashes)", expectedDir)
	}
}

func TestNew_SlashedBranchDirName(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "testrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "testrepo")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	os.Chdir(mainDir)

	if err := runApp("new", "feature/auth", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	// Directory should be feature-auth, not featureauth
	expectedDir := filepath.Join(worktreeDir, "feature-auth")
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Errorf("worktree directory not created at %s", expectedDir)
	}
}

func TestRm_RemovesWorktreeAndBranch(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "testrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "testrepo")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	os.Chdir(mainDir)

	if err := runApp("new", "to-remove", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	if err := runApp("rm", "to-remove"); err != nil {
		t.Fatalf("rm failed: %v", err)
	}

	removedDir := filepath.Join(worktreeDir, "to-remove")
	if _, err := os.Stat(removedDir); !os.IsNotExist(err) {
		t.Error("worktree directory should be removed")
	}

	// Branch should also be deleted
	bareDir := filepath.Join(home, ".willow", "repos", "testrepo.git")
	repoGit := &git.Git{Dir: bareDir}
	out, _ := repoGit.Run("branch", "--list", "to-remove")
	if out != "" {
		t.Error("branch 'to-remove' should be deleted")
	}
}

func TestRm_KeepBranch(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "testrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "testrepo")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	os.Chdir(mainDir)

	if err := runApp("new", "keep-me", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	if err := runApp("rm", "keep-me", "--keep-branch"); err != nil {
		t.Fatalf("rm --keep-branch failed: %v", err)
	}

	// Branch should still exist
	bareDir := filepath.Join(home, ".willow", "repos", "testrepo.git")
	repoGit := &git.Git{Dir: bareDir}
	out, _ := repoGit.Run("branch", "--list", "keep-me")
	if out == "" {
		t.Error("branch 'keep-me' should still exist with --keep-branch")
	}
}

func TestFilterMergedDeleteCandidates_SkipsUnsafeWorktrees(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "testrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "testrepo")
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		t.Fatalf("read worktrees dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 initial worktree, got %d", len(entries))
	}

	mainBranch := entries[0].Name()
	mainDir := filepath.Join(worktreeDir, mainBranch)
	os.Chdir(mainDir)

	mainGit := &git.Git{Dir: mainDir}
	if _, err := mainGit.Run("config", "user.email", "test@test.com"); err != nil {
		t.Fatalf("git config email: %v", err)
	}
	if _, err := mainGit.Run("config", "user.name", "Test"); err != nil {
		t.Fatalf("git config name: %v", err)
	}

	createMergedBranch := func(branch string, pushBranch, dirtyAfterMerge, unsetUpstream bool) string {
		t.Helper()

		if err := runApp("new", branch, "--no-fetch"); err != nil {
			t.Fatalf("new %s failed: %v", branch, err)
		}

		wtPath := filepath.Join(worktreeDir, branch)
		wtGit := &git.Git{Dir: wtPath}
		file := filepath.Join(wtPath, branch+".txt")
		if err := os.WriteFile(file, []byte(branch+"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
		if _, err := wtGit.Run("add", "."); err != nil {
			t.Fatalf("git add %s: %v", branch, err)
		}
		if _, err := wtGit.Run("commit", "-m", "add "+branch); err != nil {
			t.Fatalf("git commit %s: %v", branch, err)
		}
		if pushBranch {
			if _, err := wtGit.Run("push", "-u", "origin", branch); err != nil {
				t.Fatalf("git push %s: %v", branch, err)
			}
		}
		if _, err := mainGit.Run("merge", "--no-ff", branch, "-m", "merge "+branch); err != nil {
			t.Fatalf("git merge %s: %v", branch, err)
		}
		if _, err := mainGit.Run("push", "origin", mainBranch); err != nil {
			t.Fatalf("git push %s: %v", mainBranch, err)
		}
		if dirtyAfterMerge {
			if err := os.WriteFile(file, []byte(branch+"\ndirty\n"), 0o644); err != nil {
				t.Fatalf("dirty write %s: %v", file, err)
			}
		}
		if unsetUpstream {
			if _, err := wtGit.Run("branch", "--unset-upstream"); err != nil {
				t.Fatalf("unset upstream %s: %v", branch, err)
			}
		}
		return wtPath
	}

	safePath := createMergedBranch("safe-merged", true, false, false)
	dirtyPath := createMergedBranch("dirty-merged", true, true, false)
	unpushedPath := createMergedBranch("unpushed-merged", false, false, true)

	if err := runApp("new", "stack-parent", "--no-fetch"); err != nil {
		t.Fatalf("new stack-parent failed: %v", err)
	}
	parentPath := filepath.Join(worktreeDir, "stack-parent")
	parentGit := &git.Git{Dir: parentPath}
	parentFile := filepath.Join(parentPath, "stack-parent.txt")
	if err := os.WriteFile(parentFile, []byte("stack parent\n"), 0o644); err != nil {
		t.Fatalf("write stack-parent: %v", err)
	}
	if _, err := parentGit.Run("add", "."); err != nil {
		t.Fatalf("git add stack-parent: %v", err)
	}
	if _, err := parentGit.Run("commit", "-m", "add stack-parent"); err != nil {
		t.Fatalf("git commit stack-parent: %v", err)
	}
	if _, err := parentGit.Run("push", "-u", "origin", "stack-parent"); err != nil {
		t.Fatalf("git push stack-parent: %v", err)
	}

	if err := runApp("new", "stack-child", "-b", "stack-parent", "--no-fetch"); err != nil {
		t.Fatalf("new stack-child failed: %v", err)
	}
	if _, err := mainGit.Run("merge", "--no-ff", "stack-parent", "-m", "merge stack-parent"); err != nil {
		t.Fatalf("git merge stack-parent: %v", err)
	}
	if _, err := mainGit.Run("push", "origin", mainBranch); err != nil {
		t.Fatalf("git push %s: %v", mainBranch, err)
	}

	items := []tmux.PickerItem{
		{RepoName: "testrepo", Branch: "safe-merged", WtDirName: "safe-merged", WtPath: safePath},
		{RepoName: "testrepo", Branch: "dirty-merged", WtDirName: "dirty-merged", WtPath: dirtyPath},
		{RepoName: "testrepo", Branch: "unpushed-merged", WtDirName: "unpushed-merged", WtPath: unpushedPath},
		{RepoName: "testrepo", Branch: "stack-parent", WtDirName: "stack-parent", WtPath: parentPath},
	}

	safe, skipped, err := filterMergedDeleteCandidates(items)
	if err != nil {
		t.Fatalf("filterMergedDeleteCandidates failed: %v", err)
	}

	if got := branches(safe); len(got) != 1 || got[0] != "safe-merged" {
		t.Fatalf("safe branches = %v, want [safe-merged]", got)
	}

	reasons := make(map[string]string)
	for _, skip := range skipped {
		reasons[skip.Item.Branch] = skip.Reason
	}

	if !strings.Contains(reasons["dirty-merged"], "uncommitted changes") {
		t.Fatalf("dirty-merged reason = %q, want uncommitted changes", reasons["dirty-merged"])
	}
	if !strings.Contains(reasons["unpushed-merged"], "unpushed commits") {
		t.Fatalf("unpushed-merged reason = %q, want unpushed commits", reasons["unpushed-merged"])
	}
	if !strings.Contains(reasons["stack-parent"], "stacked children: stack-child") {
		t.Fatalf("stack-parent reason = %q, want stacked child", reasons["stack-parent"])
	}
}

func TestLs_ListsWorktrees(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "testrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "testrepo")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	os.Chdir(mainDir)

	if err := runApp("new", "branch-a", "--no-fetch"); err != nil {
		t.Fatalf("new branch-a failed: %v", err)
	}
	if err := runApp("new", "branch-b", "--no-fetch"); err != nil {
		t.Fatalf("new branch-b failed: %v", err)
	}

	// ls should not error (output goes to stdout, we just verify no error)
	if err := runApp("ls"); err != nil {
		t.Fatalf("ls failed: %v", err)
	}

	// ls --json should also work
	if err := runApp("ls", "--json"); err != nil {
		t.Fatalf("ls --json failed: %v", err)
	}
}

// --- Cross-repo integration tests ---

func TestSw_WithRepoFlag(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "swrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	// Create a second worktree so there's something to switch to
	worktreeDir := filepath.Join(home, ".willow", "worktrees", "swrepo")
	entries, _ := os.ReadDir(worktreeDir)
	os.Chdir(filepath.Join(worktreeDir, entries[0].Name()))
	if err := runApp("new", "sw-target", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	// Move outside the repo
	os.Chdir(home)

	// sw -r swrepo sw-target should work (direct switch by name)
	if err := runApp("sw", "-r", "swrepo", "sw-target"); err != nil {
		t.Fatalf("sw -r swrepo sw-target failed: %v", err)
	}
}

func TestSw_DirectNameFromAnywhere(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "swany"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "swany")
	entries, _ := os.ReadDir(worktreeDir)
	os.Chdir(filepath.Join(worktreeDir, entries[0].Name()))
	if err := runApp("new", "unique-branch", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	// Move outside repo
	os.Chdir(home)

	// Should find unique-branch across all repos
	if err := runApp("sw", "unique-branch"); err != nil {
		t.Fatalf("sw unique-branch from outside repo: %v", err)
	}
}

func TestStatus_WithRepoFlag(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "strepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	// Move outside repo
	os.Chdir(home)

	// status -r strepo should work
	if err := runApp("status", "-r", "strepo"); err != nil {
		t.Fatalf("status -r strepo failed: %v", err)
	}
}

func TestStatus_CrossRepoFromAnywhere(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "st1"); err != nil {
		t.Fatalf("clone st1 failed: %v", err)
	}
	if err := runApp("clone", origin, "st2"); err != nil {
		t.Fatalf("clone st2 failed: %v", err)
	}

	// Move outside repos
	os.Chdir(home)

	// status should show both repos
	if err := runApp("status"); err != nil {
		t.Fatalf("status from outside repo: %v", err)
	}
}

func TestStatus_JsonCrossRepo(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "json1"); err != nil {
		t.Fatalf("clone json1 failed: %v", err)
	}
	if err := runApp("clone", origin, "json2"); err != nil {
		t.Fatalf("clone json2 failed: %v", err)
	}

	os.Chdir(home)

	if err := runApp("status", "--json"); err != nil {
		t.Fatalf("status --json from outside repo: %v", err)
	}
}

func TestRm_WithRepoFlagFromAnywhere(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "rmrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "rmrepo")
	entries, _ := os.ReadDir(worktreeDir)
	os.Chdir(filepath.Join(worktreeDir, entries[0].Name()))
	if err := runApp("new", "rm-target", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	// Move outside repo
	os.Chdir(home)

	// rm -r rmrepo rm-target should work
	if err := runApp("rm", "-r", "rmrepo", "rm-target"); err != nil {
		t.Fatalf("rm -r rmrepo rm-target failed: %v", err)
	}

	// Verify it was removed
	removedDir := filepath.Join(worktreeDir, "rm-target")
	if _, err := os.Stat(removedDir); !os.IsNotExist(err) {
		t.Error("worktree should be removed")
	}
}

func TestRm_CrossRepoByNameFromAnywhere(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "rmany"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "rmany")
	entries, _ := os.ReadDir(worktreeDir)
	os.Chdir(filepath.Join(worktreeDir, entries[0].Name()))
	if err := runApp("new", "cross-rm-branch", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	os.Chdir(home)

	// Should find cross-rm-branch across repos and remove it
	if err := runApp("rm", "cross-rm-branch"); err != nil {
		t.Fatalf("rm cross-rm-branch from outside repo: %v", err)
	}

	removedDir := filepath.Join(worktreeDir, "cross-rm-branch")
	if _, err := os.Stat(removedDir); !os.IsNotExist(err) {
		t.Error("worktree should be removed")
	}
}

func TestLs_RepoListShowsActiveUnread(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "lsrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	// Move outside repo to trigger repo list mode
	os.Chdir(home)

	// ls from outside repo should show repo list with columns (no error)
	if err := runApp("ls"); err != nil {
		t.Fatalf("ls from outside repo: %v", err)
	}
}

func TestSw_ScopedToCurrentRepo(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "scoped"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "scoped")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	os.Chdir(mainDir)

	if err := runApp("new", "scoped-branch", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	// sw from inside repo should still work (scoped to current repo)
	if err := runApp("sw", "scoped-branch"); err != nil {
		t.Fatalf("sw scoped-branch from inside repo: %v", err)
	}
}

func TestStatus_ScopedToCurrentRepo(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "stscoped"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "stscoped")
	entries, _ := os.ReadDir(worktreeDir)
	os.Chdir(filepath.Join(worktreeDir, entries[0].Name()))

	// status from inside repo should still be scoped
	if err := runApp("status"); err != nil {
		t.Fatalf("status from inside repo: %v", err)
	}
}

func TestRm_ScopedToCurrentRepo(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "rmscoped"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "rmscoped")
	entries, _ := os.ReadDir(worktreeDir)
	os.Chdir(filepath.Join(worktreeDir, entries[0].Name()))

	if err := runApp("new", "scoped-rm", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	// rm from inside repo should be scoped
	if err := runApp("rm", "scoped-rm"); err != nil {
		t.Fatalf("rm scoped-rm from inside repo: %v", err)
	}

	removedDir := filepath.Join(worktreeDir, "scoped-rm")
	if _, err := os.Stat(removedDir); !os.IsNotExist(err) {
		t.Error("worktree should be removed")
	}
}

func TestNew_ExistingBranch(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "testrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	// Create a branch in the origin (push from the initial worktree)
	worktreeDir := filepath.Join(home, ".willow", "worktrees", "testrepo")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())

	wg := &git.Git{Dir: mainDir}
	if _, err := wg.Run("config", "user.email", "test@test.com"); err != nil {
		t.Fatalf("git config email: %v", err)
	}
	if _, err := wg.Run("config", "user.name", "Test"); err != nil {
		t.Fatalf("git config name: %v", err)
	}
	if _, err := wg.Run("checkout", "-b", "existing-feature"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	os.WriteFile(filepath.Join(mainDir, "feature.txt"), []byte("feature\n"), 0o644)
	if _, err := wg.Run("add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := wg.Run("commit", "-m", "add feature"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	if _, err := wg.Run("push", "origin", "existing-feature"); err != nil {
		t.Fatalf("git push: %v", err)
	}
	// Switch back so the branch isn't checked out in this worktree
	if _, err := wg.Run("checkout", "-"); err != nil {
		t.Fatalf("git checkout: %v", err)
	}

	os.Chdir(mainDir)

	if err := runApp("new", "-e", "existing-feature", "--no-fetch"); err != nil {
		t.Fatalf("new -e failed: %v", err)
	}

	newWtDir := filepath.Join(worktreeDir, "existing-feature")
	if _, err := os.Stat(newWtDir); os.IsNotExist(err) {
		t.Errorf("worktree not created at %s", newWtDir)
	}

	// Verify the feature file exists (branch content was checked out)
	if _, err := os.Stat(filepath.Join(newWtDir, "feature.txt")); os.IsNotExist(err) {
		t.Error("feature.txt not found — existing branch content not checked out")
	}
}

func TestNew_ExistingBranchSkipsPrefix(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "testrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "testrepo")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())

	// Create and push a branch
	wg := &git.Git{Dir: mainDir}
	if _, err := wg.Run("checkout", "-b", "prefixed-branch"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	if _, err := wg.Run("push", "origin", "prefixed-branch"); err != nil {
		t.Fatalf("git push: %v", err)
	}
	if _, err := wg.Run("checkout", "-"); err != nil {
		t.Fatalf("git checkout: %v", err)
	}

	os.Chdir(mainDir)

	// Set branchPrefix in local config
	bareDir := filepath.Join(home, ".willow", "repos", "testrepo.git")
	configPath := filepath.Join(bareDir, "willow.json")
	os.WriteFile(configPath, []byte(`{"branchPrefix": "alice"}`), 0o644)

	if err := runApp("new", "-e", "prefixed-branch", "--no-fetch"); err != nil {
		t.Fatalf("new -e failed: %v", err)
	}

	// Directory should be "prefixed-branch", NOT "alice-prefixed-branch"
	expectedDir := filepath.Join(worktreeDir, "prefixed-branch")
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Errorf("worktree not at %s — branch prefix was wrongly applied", expectedDir)
	}

	wrongDir := filepath.Join(worktreeDir, "alice-prefixed-branch")
	if _, err := os.Stat(wrongDir); !os.IsNotExist(err) {
		t.Error("branch prefix was applied to existing branch (should be skipped)")
	}
}

func TestIsPRURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https://github.com/org/repo/pull/123", true},
		{"github.com/org/repo/pull/456", true},
		{"https://github.com/org/repo/pull/123#issuecomment-1", true},
		{"feat-my-branch", false},
		{"https://github.com/org/repo/issues/123", false},
		{"", false},
		{"123", false},
	}
	for _, tt := range tests {
		if got := isPRURL(tt.input); got != tt.want {
			t.Errorf("isPRURL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestNew_StackedBranch(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "testrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "testrepo")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	os.Chdir(mainDir)

	// Create feature-a from main
	if err := runApp("new", "feature-a", "--no-fetch"); err != nil {
		t.Fatalf("new feature-a failed: %v", err)
	}

	// Create feature-b stacked on feature-a
	if err := runApp("new", "feature-b", "-b", "feature-a", "--no-fetch"); err != nil {
		t.Fatalf("new feature-b -b feature-a failed: %v", err)
	}

	// Verify worktrees exist
	if _, err := os.Stat(filepath.Join(worktreeDir, "feature-a")); os.IsNotExist(err) {
		t.Error("feature-a worktree not created")
	}
	if _, err := os.Stat(filepath.Join(worktreeDir, "feature-b")); os.IsNotExist(err) {
		t.Error("feature-b worktree not created")
	}

	// Verify stack was recorded
	bareDir := filepath.Join(home, ".willow", "repos", "testrepo.git")
	data, err := os.ReadFile(filepath.Join(bareDir, "branches.json"))
	if err != nil {
		t.Fatalf("branches.json not found: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "feature-a") || !strings.Contains(content, "feature-b") {
		t.Errorf("branches.json missing expected entries: %s", content)
	}
}

func TestCheckout_SwitchesToExistingWorktree(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "testrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "testrepo")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	os.Chdir(mainDir)

	if err := runApp("new", "branch-a", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	// checkout should switch to the existing worktree (print its path)
	if err := runApp("checkout", "branch-a", "--cd"); err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
}

func TestCheckout_CreatesWorktreeForExistingBranch(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "testrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "testrepo")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())

	// Create and push a branch to origin
	wg := &git.Git{Dir: mainDir}
	if _, err := wg.Run("config", "user.email", "test@test.com"); err != nil {
		t.Fatalf("git config email: %v", err)
	}
	if _, err := wg.Run("config", "user.name", "Test"); err != nil {
		t.Fatalf("git config name: %v", err)
	}
	if _, err := wg.Run("checkout", "-b", "remote-only"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	os.WriteFile(filepath.Join(mainDir, "remote.txt"), []byte("remote\n"), 0o644)
	if _, err := wg.Run("add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := wg.Run("commit", "-m", "remote commit"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	if _, err := wg.Run("push", "origin", "remote-only"); err != nil {
		t.Fatalf("git push: %v", err)
	}
	if _, err := wg.Run("checkout", "-"); err != nil {
		t.Fatalf("git checkout: %v", err)
	}

	os.Chdir(mainDir)

	// checkout should detect the remote branch and create a worktree
	if err := runApp("checkout", "remote-only", "--no-fetch"); err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	newWtDir := filepath.Join(worktreeDir, "remote-only")
	if _, err := os.Stat(newWtDir); os.IsNotExist(err) {
		t.Errorf("worktree not created at %s", newWtDir)
	}

	// Verify content
	if _, err := os.Stat(filepath.Join(newWtDir, "remote.txt")); os.IsNotExist(err) {
		t.Error("remote.txt not found — existing branch content not checked out")
	}
}

func TestCheckout_CreatesNewBranch(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "testrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "testrepo")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	os.Chdir(mainDir)

	if err := runApp("checkout", "brand-new", "--no-fetch"); err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	newWtDir := filepath.Join(worktreeDir, "brand-new")
	if _, err := os.Stat(newWtDir); os.IsNotExist(err) {
		t.Errorf("worktree not created at %s", newWtDir)
	}

	// Verify the branch was created
	bareDir := filepath.Join(home, ".willow", "repos", "testrepo.git")
	repoGit := &git.Git{Dir: bareDir}
	out, err := repoGit.Run("branch", "--list", "brand-new")
	if err != nil {
		t.Fatalf("git branch --list: %v", err)
	}
	if out == "" {
		t.Error("branch 'brand-new' not found")
	}
}

func TestGc_EmptyTrash(t *testing.T) {
	setupTestEnv(t)

	if err := runApp("gc"); err != nil {
		t.Fatalf("gc failed: %v", err)
	}
}

func TestGc_CleansTrash(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "gcrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "gcrepo")
	entries, _ := os.ReadDir(worktreeDir)
	os.Chdir(filepath.Join(worktreeDir, entries[0].Name()))

	if err := runApp("new", "gc-branch", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}
	if err := runApp("rm", "gc-branch"); err != nil {
		t.Fatalf("rm failed: %v", err)
	}

	if err := runApp("gc"); err != nil {
		t.Fatalf("gc failed: %v", err)
	}

	trashDir := filepath.Join(home, ".willow", "trash")
	trashEntries, err := os.ReadDir(trashDir)
	if err == nil && len(trashEntries) > 0 {
		t.Errorf("trash dir should be empty after gc, has %d entries", len(trashEntries))
	}
}

func TestMergedWorktrees_UsesGitHubMergedPRsAcrossCLI(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "mergedrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "mergedrepo")
	entries, _ := os.ReadDir(worktreeDir)
	baseBranch := entries[0].Name()
	mainDir := filepath.Join(worktreeDir, baseBranch)
	os.Chdir(mainDir)

	if err := runApp("new", "feature-merged", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	featureDir := filepath.Join(worktreeDir, "feature-merged")
	commitFile(t, featureDir, "feature.txt", "feature\n", "add feature")

	bareDir := filepath.Join(home, ".willow", "repos", "mergedrepo.git")
	repoGit := &git.Git{Dir: bareDir}
	gitMerged := repoGit.MergedBranchSet(baseBranch, []string{"feature-merged"})
	if gitMerged["feature-merged"] {
		t.Fatalf("feature-merged should not be git-merged in this regression setup, got %v", gitMerged)
	}

	headOID, err := (&git.Git{Dir: featureDir}).Run("rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	installMergedStatusGH(t, baseBranch, "feature-merged", strings.TrimSpace(headOID))

	lsOut, err := captureStdout(t, func() error {
		return runApp("ls", "mergedrepo")
	})
	if err != nil {
		t.Fatalf("ls failed: %v", err)
	}
	if !strings.Contains(lsOut, "feature-merged") || !strings.Contains(lsOut, "[merged]") {
		t.Fatalf("expected ls output to include merged feature worktree, got:\n%s", lsOut)
	}

	gcOut, err := captureStdout(t, func() error {
		return runApp("gc")
	})
	if err != nil {
		t.Fatalf("gc failed: %v", err)
	}
	if !strings.Contains(gcOut, "feature-merged (repo: mergedrepo)") {
		t.Fatalf("expected gc output to include merged feature worktree, got:\n%s", gcOut)
	}

	wts, err := worktree.List(repoGit)
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	mergedSet := mergedBranchSetForRepo("mergedrepo", bareDir, filterBareWorktrees(wts))
	if !mergedSet["feature-merged"] {
		t.Fatalf("expected sw merged helper to include feature-merged, got %v", mergedSet)
	}
}

func TestNew_WithRepoFlag(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "newrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	os.Chdir(home)

	if err := runApp("new", "remote-branch", "-r", "newrepo", "--no-fetch"); err != nil {
		t.Fatalf("new -r newrepo remote-branch failed: %v", err)
	}

	newWtDir := filepath.Join(home, ".willow", "worktrees", "newrepo", "remote-branch")
	if _, err := os.Stat(newWtDir); os.IsNotExist(err) {
		t.Errorf("worktree not created at %s", newWtDir)
	}
}

func TestLs_PathOnly(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "pathrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "pathrepo")
	entries, _ := os.ReadDir(worktreeDir)
	os.Chdir(filepath.Join(worktreeDir, entries[0].Name()))

	if err := runApp("ls", "--path-only"); err != nil {
		t.Fatalf("ls --path-only failed: %v", err)
	}
}

func TestLs_WithRepoArg(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "lsarg"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	os.Chdir(home)

	if err := runApp("ls", "lsarg"); err != nil {
		t.Fatalf("ls lsarg failed: %v", err)
	}
}

func TestClone_UsesCustomBaseDirFromEnv(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, "custom-willow-base")
	t.Setenv("WILLOW_BASE_DIR", baseDir)

	if err := runApp("clone", origin, "envrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	bareDir := filepath.Join(baseDir, "repos", "envrepo.git")
	if _, err := os.Stat(bareDir); os.IsNotExist(err) {
		t.Fatalf("bare repo not created at %s", bareDir)
	}

	worktreeDir := filepath.Join(baseDir, "worktrees", "envrepo")
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		t.Fatalf("read custom worktrees dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(entries))
	}
}

func TestNewAndCheckout_UseCustomBaseDirFromGlobalConfig(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, "custom global willow")
	writeGlobalConfigFile(t, `{"baseDir":"~/custom global willow"}`)

	if err := runApp("clone", origin, "customrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(baseDir, "worktrees", "customrepo")
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		t.Fatalf("read custom worktrees dir: %v", err)
	}
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	if err := os.Chdir(mainDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := runApp("new", "custom-new", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktreeDir, "custom-new")); os.IsNotExist(err) {
		t.Fatalf("new worktree missing under custom base")
	}

	if err := runApp("checkout", "custom-checkout", "--no-fetch"); err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktreeDir, "custom-checkout")); os.IsNotExist(err) {
		t.Fatalf("checkout worktree missing under custom base")
	}
}

func TestMigrateBase_DryRunDoesNotChangeState(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "dryrunrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	sourceBase := filepath.Join(home, ".willow")
	destBase := filepath.Join(home, "migrated", "willow")
	if err := os.Chdir(home); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := runApp("migrate-base", destBase, "--dry-run"); err != nil {
		t.Fatalf("migrate-base --dry-run failed: %v", err)
	}

	if _, err := os.Stat(sourceBase); os.IsNotExist(err) {
		t.Fatalf("source base should still exist after dry-run")
	}
	if _, err := os.Stat(destBase); !os.IsNotExist(err) {
		t.Fatalf("destination should not exist after dry-run")
	}
}

func TestMigrateBase_AllowsHookOnlyWillowBase(t *testing.T) {
	setupTestEnv(t)
	home, _ := os.UserHomeDir()

	statusDir := filepath.Join(home, ".willow", "status")
	if err := os.MkdirAll(statusDir, 0o755); err != nil {
		t.Fatalf("mkdir status dir: %v", err)
	}

	destBase := filepath.Join(home, "hook-only-willow")
	if err := os.Chdir(home); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := runApp("migrate-base", destBase, "--yes"); err != nil {
		t.Fatalf("migrate-base failed for hook-only base: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destBase, "status")); err != nil {
		t.Fatalf("status dir missing after migration: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".willow")); !os.IsNotExist(err) {
		t.Fatalf("old base should be removed")
	}
}

func TestMigrateBase_MovesRepoAndRepairsWorktrees(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "moverepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	oldWorktreeDir := filepath.Join(home, ".willow", "worktrees", "moverepo")
	entries, err := os.ReadDir(oldWorktreeDir)
	if err != nil {
		t.Fatalf("read old worktree dir: %v", err)
	}
	mainDir := filepath.Join(oldWorktreeDir, entries[0].Name())
	if err := os.Chdir(mainDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := runApp("new", "feature-migrate", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	destBase := filepath.Join(home, "moved-willow")
	if err := os.Chdir(home); err != nil {
		t.Fatalf("chdir home: %v", err)
	}

	if err := runApp("migrate-base", destBase, "--yes"); err != nil {
		t.Fatalf("migrate-base failed: %v", err)
	}

	if got := config.WillowHome(); got != destBase {
		t.Fatalf("WillowHome() = %q, want %q", got, destBase)
	}
	if _, err := os.Stat(filepath.Join(home, ".willow")); !os.IsNotExist(err) {
		t.Fatalf("old base should be removed")
	}

	newBareDir := filepath.Join(destBase, "repos", "moverepo.git")
	newWorktree := filepath.Join(destBase, "worktrees", "moverepo", "feature-migrate")
	if _, err := os.Stat(newBareDir); os.IsNotExist(err) {
		t.Fatalf("new bare repo missing at %s", newBareDir)
	}
	if _, err := os.Stat(newWorktree); os.IsNotExist(err) {
		t.Fatalf("new worktree missing at %s", newWorktree)
	}
	if _, err := os.Stat(filepath.Join(newBareDir, "branches.json")); os.IsNotExist(err) {
		t.Fatalf("branches.json should move with the bare repo")
	}

	repoGit := &git.Git{Dir: newBareDir}
	if _, err := repoGit.Run("worktree", "list", "--porcelain"); err != nil {
		t.Fatalf("git worktree list failed after migration: %v", err)
	}

	wtGit := &git.Git{Dir: newWorktree}
	top, err := wtGit.Run("rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatalf("git rev-parse failed after migration: %v", err)
	}
	if comparablePath(top) != comparablePath(newWorktree) {
		t.Fatalf("rev-parse top-level = %q, want %q", top, newWorktree)
	}
}

func TestMigrateBase_RejectsNonEmptyDestination(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "rejectdest"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	destBase := filepath.Join(home, "occupied")
	if err := os.MkdirAll(destBase, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destBase, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write dest file: %v", err)
	}
	if err := os.Chdir(home); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := runApp("migrate-base", destBase, "--yes"); err == nil {
		t.Fatal("expected migrate-base to reject a non-empty destination")
	}
}

func TestMigrateBase_RejectsCwdInsideOldBase(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "insidecwd"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "insidecwd")
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		t.Fatalf("read worktrees dir: %v", err)
	}
	if err := os.Chdir(filepath.Join(worktreeDir, entries[0].Name())); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	destBase := filepath.Join(home, "new-base")
	if err := runApp("migrate-base", destBase, "--yes"); err == nil {
		t.Fatal("expected migrate-base to reject running from inside the old base")
	}
}

func TestMigrateBase_RejectsActiveSessions(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "sessionrepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "sessionrepo")
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		t.Fatalf("read worktrees dir: %v", err)
	}
	writeActiveSessionFile(t, "sessionrepo", entries[0].Name(), "s1", claude.StatusDone)
	if err := os.Chdir(home); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	destBase := filepath.Join(home, "blocked-base")
	if err := runApp("migrate-base", destBase, "--yes"); err == nil {
		t.Fatal("expected migrate-base to reject active Claude session files")
	}
}
