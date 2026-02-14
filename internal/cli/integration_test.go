package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/iamrajjoshi/willow/internal/git"
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

	if err := runApp("rm", "to-remove", "--yes"); err != nil {
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

	if err := runApp("rm", "keep-me", "--yes", "--keep-branch"); err != nil {
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
