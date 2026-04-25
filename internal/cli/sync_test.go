package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/git"
)

type syncStackFixture struct {
	Home        string
	BareDir     string
	WorktreeDir string
	BaseBranch  string
	MainDir     string
	FeatureADir string
	FeatureBDir string
}

func setupSyncStack(t *testing.T, repoName string) syncStackFixture {
	t.Helper()

	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	if err := runApp("clone", origin, repoName); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", repoName)
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		t.Fatalf("read worktrees dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one initial worktree, got %d", len(entries))
	}
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	if err := os.Chdir(mainDir); err != nil {
		t.Fatalf("chdir main: %v", err)
	}

	if err := runApp("new", "feature-a", "--no-fetch"); err != nil {
		t.Fatalf("new feature-a failed: %v", err)
	}
	featureADir := filepath.Join(worktreeDir, "feature-a")
	commitFile(t, featureADir, "feature-a.txt", "feature a\n", "feature a")

	if err := os.Chdir(mainDir); err != nil {
		t.Fatalf("chdir main: %v", err)
	}
	if err := runApp("new", "feature-b", "--base", "feature-a", "--no-fetch"); err != nil {
		t.Fatalf("new feature-b failed: %v", err)
	}
	featureBDir := filepath.Join(worktreeDir, "feature-b")
	commitFile(t, featureBDir, "feature-b.txt", "feature b\n", "feature b")

	return syncStackFixture{
		Home:        home,
		BareDir:     filepath.Join(home, ".willow", "repos", repoName+".git"),
		WorktreeDir: worktreeDir,
		BaseBranch:  entries[0].Name(),
		MainDir:     mainDir,
		FeatureADir: featureADir,
		FeatureBDir: featureBDir,
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := (&git.Git{Dir: dir}).Run(args...)
	if err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
	return strings.TrimSpace(out)
}

func TestSync_NoStack(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	if err := runApp("clone", origin, "nostack"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	worktreeDir := filepath.Join(home, ".willow", "worktrees", "nostack")
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		t.Fatalf("read worktrees dir: %v", err)
	}
	if err := os.Chdir(filepath.Join(worktreeDir, entries[0].Name())); err != nil {
		t.Fatalf("chdir worktree: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runApp("sync", "--no-fetch")
	})
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if !strings.Contains(out, "No stacked branches found") {
		t.Fatalf("expected no-stack message, got:\n%s", out)
	}
}

func TestSync_RebasesChildOntoUpdatedParent(t *testing.T) {
	f := setupSyncStack(t, "syncfull")
	commitFile(t, f.FeatureADir, "feature-a-2.txt", "feature a update\n", "feature a update")

	out, err := captureStdout(t, func() error {
		return runApp("sync", "--no-fetch")
	})
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if !strings.Contains(out, "All 2 worktree(s) synced") {
		t.Fatalf("expected successful sync summary, got:\n%s", out)
	}

	if _, err := (&git.Git{Dir: f.FeatureBDir}).Run("merge-base", "--is-ancestor", "feature-a", "HEAD"); err != nil {
		t.Fatalf("feature-b should be rebased onto feature-a: %v", err)
	}
}

func TestSync_TargetBranchOnlySyncsSubtree(t *testing.T) {
	f := setupSyncStack(t, "syncsubtree")
	if err := os.Chdir(f.MainDir); err != nil {
		t.Fatalf("chdir main: %v", err)
	}
	if err := runApp("new", "feature-c", "--base", "feature-b", "--no-fetch"); err != nil {
		t.Fatalf("new feature-c failed: %v", err)
	}
	featureCDir := filepath.Join(f.WorktreeDir, "feature-c")
	commitFile(t, featureCDir, "feature-c.txt", "feature c\n", "feature c")
	commitFile(t, f.FeatureBDir, "feature-b-2.txt", "feature b update\n", "feature b update")

	out, err := captureStdout(t, func() error {
		return runApp("sync", "feature-b", "--no-fetch")
	})
	if err != nil {
		t.Fatalf("sync feature-b failed: %v", err)
	}
	if !strings.Contains(out, "Syncing 2 stacked worktree(s)") {
		t.Fatalf("expected subtree sync count, got:\n%s", out)
	}
	if _, err := (&git.Git{Dir: featureCDir}).Run("merge-base", "--is-ancestor", "feature-b", "HEAD"); err != nil {
		t.Fatalf("feature-c should be rebased onto updated feature-b: %v", err)
	}
}

func TestSync_SkipsTrackedBranchWithoutWorktree(t *testing.T) {
	f := setupSyncStack(t, "syncmissing")
	if _, err := (&git.Git{Dir: f.BareDir}).Run("worktree", "remove", "--force", f.FeatureBDir); err != nil {
		t.Fatalf("remove feature-b worktree: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runApp("sync", "--no-fetch")
	})
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if !strings.Contains(out, "Skipped (no worktree)") {
		t.Fatalf("expected missing-worktree skip, got:\n%s", out)
	}
}

func TestSync_SkipsDirtyWorktree(t *testing.T) {
	f := setupSyncStack(t, "syncdirty")
	oldHead := gitOutput(t, f.FeatureBDir, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(f.FeatureBDir, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runApp("sync", "--no-fetch")
	})
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if !strings.Contains(out, "Skipped (uncommitted changes)") {
		t.Fatalf("expected dirty-worktree skip, got:\n%s", out)
	}
	if got := gitOutput(t, f.FeatureBDir, "rev-parse", "HEAD"); got != oldHead {
		t.Fatalf("dirty worktree head changed: got %s, want %s", got, oldHead)
	}
}

func TestSync_ConflictSkipsDescendantsAndAbortClearsRebase(t *testing.T) {
	f := setupSyncStack(t, "syncconflict")

	commitFile(t, f.FeatureADir, "conflict.txt", "base\n", "add conflict base")
	if err := os.Chdir(f.MainDir); err != nil {
		t.Fatalf("chdir main: %v", err)
	}
	if err := runApp("new", "conflict-child", "--base", "feature-a", "--no-fetch"); err != nil {
		t.Fatalf("new conflict-child failed: %v", err)
	}
	childDir := filepath.Join(f.WorktreeDir, "conflict-child")
	commitFile(t, childDir, "conflict.txt", "child\n", "child conflict edit")
	if err := os.Chdir(f.MainDir); err != nil {
		t.Fatalf("chdir main: %v", err)
	}
	if err := runApp("new", "conflict-grandchild", "--base", "conflict-child", "--no-fetch"); err != nil {
		t.Fatalf("new conflict-grandchild failed: %v", err)
	}
	grandchildDir := filepath.Join(f.WorktreeDir, "conflict-grandchild")
	commitFile(t, grandchildDir, "grandchild.txt", "grandchild\n", "grandchild work")
	commitFile(t, f.FeatureADir, "conflict.txt", "parent\n", "parent conflict edit")

	out, err := captureStdout(t, func() error {
		return runApp("sync", "--no-fetch")
	})
	if err != nil {
		t.Fatalf("sync should report conflicts without returning an error: %v", err)
	}
	if !strings.Contains(out, "Conflict") {
		t.Fatalf("expected conflict output, got:\n%s", out)
	}
	if !strings.Contains(out, "Skipped (ancestor has conflict)") {
		t.Fatalf("expected descendant skip after conflict, got:\n%s", out)
	}
	if !(&git.Git{Dir: childDir}).IsRebaseInProgress() {
		t.Fatal("expected conflict-child to have an in-progress rebase")
	}

	abortOut, err := captureStdout(t, func() error {
		return runApp("sync", "--abort", "--no-fetch")
	})
	if err != nil {
		t.Fatalf("sync --abort failed: %v", err)
	}
	if !strings.Contains(abortOut, "Aborted 1 rebase") {
		t.Fatalf("expected abort summary, got:\n%s", abortOut)
	}
	if (&git.Git{Dir: childDir}).IsRebaseInProgress() {
		t.Fatal("expected sync --abort to clear the in-progress rebase")
	}
}
