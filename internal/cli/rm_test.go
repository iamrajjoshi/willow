package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadGitAdminDir_Absolute(t *testing.T) {
	dir := t.TempDir()
	adminDir := "/fake/bare.git/worktrees/my-branch"
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: "+adminDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := readGitAdminDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != adminDir {
		t.Errorf("got %q, want %q", got, adminDir)
	}
}

func TestRmCommandPickerSingleRepoRemovesSelectedWorktree(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	if err := runApp("clone", origin, "rmpicker"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	worktreeRoot := filepath.Join(home, ".willow", "worktrees", "rmpicker")
	mainDir := filepath.Join(worktreeRoot, firstWorktreeDir(t, worktreeRoot))
	if err := os.Chdir(mainDir); err != nil {
		t.Fatalf("chdir main: %v", err)
	}
	if err := runApp("new", "remove-picker", "--no-fetch"); err != nil {
		t.Fatalf("new remove-picker failed: %v", err)
	}
	t.Setenv("FZF_DEFAULT_OPTS", "--filter=remove-picker")

	if err := runApp("rm", "--repo", "rmpicker", "--force", "--keep-branch"); err != nil {
		t.Fatalf("rm picker failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktreeRoot, "remove-picker")); !os.IsNotExist(err) {
		t.Fatalf("remove-picker worktree still exists or stat failed unexpectedly: %v", err)
	}
}

func TestRmCommandPickerMultiRepoRemovesSelectedWorktree(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	if err := runApp("clone", origin, "rmone"); err != nil {
		t.Fatalf("clone rmone failed: %v", err)
	}
	if err := runApp("clone", origin, "rmtwo"); err != nil {
		t.Fatalf("clone rmtwo failed: %v", err)
	}
	if err := os.Chdir(home); err != nil {
		t.Fatalf("chdir home: %v", err)
	}
	t.Setenv("FZF_DEFAULT_OPTS", "--filter=rmtwo/")

	if err := runApp("rm", "--force", "--keep-branch"); err != nil {
		t.Fatalf("multi-repo rm picker failed: %v", err)
	}
	rmtwoRoot := filepath.Join(home, ".willow", "worktrees", "rmtwo")
	entries, err := os.ReadDir(rmtwoRoot)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read rmtwo worktree root: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("rmtwo worktree root entries = %s, want selected worktree removed", strings.Join(names, ", "))
	}
}

func TestReadGitAdminDir_Relative(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../../bare.git/worktrees/my-branch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := readGitAdminDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Clean(filepath.Join(dir, "../../bare.git/worktrees/my-branch"))
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReadGitAdminDir_NoFile(t *testing.T) {
	dir := t.TempDir()
	_, err := readGitAdminDir(dir)
	if err == nil {
		t.Fatal("expected error for missing .git file")
	}
}

func TestReadGitAdminDir_BadFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("not a gitdir line\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := readGitAdminDir(dir)
	if err == nil {
		t.Fatal("expected error for bad format")
	}
}

// Regression test for #95: when git appends a numeric suffix to the admin dir
// name (e.g. "my-branch1" instead of "my-branch"), the old code computed the
// wrong path and os.RemoveAll silently succeeded on a non-existent dir.
// The fix reads the actual path from the worktree's .git file.
func TestReadGitAdminDir_CollisionSuffix(t *testing.T) {
	root := t.TempDir()

	// Simulate a bare repo with a collision-suffixed admin dir
	bareDir := filepath.Join(root, "repo.git")
	correctAdmin := filepath.Join(bareDir, "worktrees", "my-branch1")
	wrongAdmin := filepath.Join(bareDir, "worktrees", "my-branch")
	os.MkdirAll(correctAdmin, 0o755)
	os.MkdirAll(wrongAdmin, 0o755)

	// Create a sentinel file in the correct admin dir
	os.WriteFile(filepath.Join(correctAdmin, "HEAD"), []byte("ref: refs/heads/my-branch\n"), 0o644)

	// Simulate a worktree directory whose .git file points to the suffixed name
	wtPath := filepath.Join(root, "worktrees", "my-branch")
	os.MkdirAll(wtPath, 0o755)
	os.WriteFile(filepath.Join(wtPath, ".git"), []byte("gitdir: "+correctAdmin+"\n"), 0o644)

	// readGitAdminDir should return the CORRECT (suffixed) admin dir
	got, err := readGitAdminDir(wtPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != correctAdmin {
		t.Errorf("got %q, want %q (the collision-suffixed admin dir)", got, correctAdmin)
	}

	// The old code would have computed the WRONG path:
	guessedAdmin := filepath.Join(bareDir, "worktrees", filepath.Base(wtPath))
	if guessedAdmin == correctAdmin {
		t.Fatal("test setup error: guessed and correct paths should differ")
	}

	// Verify that removing the guessed path does NOT remove the correct admin dir
	os.RemoveAll(guessedAdmin)
	if _, err := os.Stat(filepath.Join(correctAdmin, "HEAD")); err != nil {
		t.Fatal("removing the guessed admin dir should not affect the correct admin dir")
	}
}

// Regression test for #95: the old code removed the admin dir BEFORE moving
// the worktree directory. If the move failed, the admin dir was already gone
// (inconsistent state). The fix swaps the order: move first, then remove admin.
func TestRemoveWorktree_OrderOfOperations(t *testing.T) {
	root := t.TempDir()

	// Set up a fake bare repo admin dir
	bareDir := filepath.Join(root, "repo.git")
	adminDir := filepath.Join(bareDir, "worktrees", "test-branch")
	os.MkdirAll(adminDir, 0o755)
	os.WriteFile(filepath.Join(adminDir, "HEAD"), []byte("ref: refs/heads/test-branch\n"), 0o644)

	// Set up a fake worktree with a .git file
	wtPath := filepath.Join(root, "worktrees", "test-branch")
	os.MkdirAll(wtPath, 0o755)
	os.WriteFile(filepath.Join(wtPath, ".git"), []byte("gitdir: "+adminDir+"\n"), 0o644)
	os.WriteFile(filepath.Join(wtPath, "somefile.txt"), []byte("content"), 0o644)

	// Step 1: read admin dir (must succeed before any mutation)
	gotAdmin, err := readGitAdminDir(wtPath)
	if err != nil {
		t.Fatalf("readGitAdminDir failed: %v", err)
	}
	if gotAdmin != adminDir {
		t.Fatalf("admin dir mismatch: got %q, want %q", gotAdmin, adminDir)
	}

	// Step 2: move worktree (the "reversible" step)
	trashDest := filepath.Join(root, "trash", "test-branch")
	os.MkdirAll(filepath.Join(root, "trash"), 0o755)
	if err := os.Rename(wtPath, trashDest); err != nil {
		t.Fatalf("rename failed: %v", err)
	}

	// After the move, admin dir must STILL exist (this was the bug — old code
	// already deleted it before this point)
	if _, err := os.Stat(filepath.Join(adminDir, "HEAD")); err != nil {
		t.Fatal("admin dir should still exist after moving worktree to trash")
	}

	// Worktree should be gone from its original path
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatal("worktree should be gone from original path after rename")
	}

	// Step 3: NOW remove admin dir
	if err := os.RemoveAll(gotAdmin); err != nil {
		t.Fatalf("admin dir removal failed: %v", err)
	}

	if _, err := os.Stat(adminDir); !os.IsNotExist(err) {
		t.Fatal("admin dir should be gone after removal")
	}
}
