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
