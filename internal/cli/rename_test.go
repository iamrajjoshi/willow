package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/worktree"
)

type renameTestRepo struct {
	Origin      string
	Home        string
	RepoName    string
	WorktreeDir string
	MainDir     string
	BareDir     string
}

func setupRenameTestRepo(t *testing.T, repoName string) renameTestRepo {
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
	if len(entries) == 0 {
		t.Fatalf("expected initial worktree")
	}
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	if err := os.Chdir(mainDir); err != nil {
		t.Fatalf("chdir main: %v", err)
	}

	return renameTestRepo{
		Origin:      origin,
		Home:        home,
		RepoName:    repoName,
		WorktreeDir: worktreeDir,
		MainDir:     mainDir,
		BareDir:     filepath.Join(home, ".willow", "repos", repoName+".git"),
	}
}

func TestRename_CurrentBranchBackedWorktree(t *testing.T) {
	repo := setupRenameTestRepo(t, "renamecurrent")
	if err := runApp("new", "old", "--no-fetch"); err != nil {
		t.Fatalf("new old failed: %v", err)
	}

	oldPath := filepath.Join(repo.WorktreeDir, "old")
	nested := filepath.Join(oldPath, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatalf("chdir nested: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runApp("rename", "new", "--cd")
	})
	if err != nil {
		t.Fatalf("rename failed: %v", err)
	}

	newPath := filepath.Join(repo.WorktreeDir, "new")
	if got, want := comparablePath(strings.TrimSpace(out)), comparablePath(filepath.Join(newPath, "nested")); got != want {
		t.Fatalf("rename --cd output = %q, want %q", got, want)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new worktree path missing: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old worktree path should be gone, err=%v", err)
	}

	repoGit := &git.Git{Dir: repo.BareDir}
	if out, _ := repoGit.Run("branch", "--list", "old"); out != "" {
		t.Fatalf("old branch should not remain: %s", out)
	}
	if out, _ := repoGit.Run("branch", "--list", "new"); out == "" {
		t.Fatalf("new branch should exist")
	}
	wts, err := worktree.List(repoGit)
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	renamed, err := findWorktree(filterBareWorktrees(wts), "new")
	if err != nil {
		t.Fatalf("renamed worktree not found: %v", err)
	}
	if comparablePath(renamed.Path) != comparablePath(newPath) {
		t.Fatalf("renamed path = %q, want %q", renamed.Path, newPath)
	}
}

func TestRename_SelectedWorktreeWithRepo(t *testing.T) {
	repo := setupRenameTestRepo(t, "renameselected")
	if err := runApp("new", "old", "--no-fetch"); err != nil {
		t.Fatalf("new old failed: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}

	if err := runApp("rename", "--repo", repo.RepoName, "old", "selected-new"); err != nil {
		t.Fatalf("rename selected failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repo.WorktreeDir, "selected-new")); err != nil {
		t.Fatalf("selected-new worktree missing: %v", err)
	}
	if out, _ := (&git.Git{Dir: repo.BareDir}).Run("branch", "--list", "selected-new"); out == "" {
		t.Fatalf("selected-new branch should exist")
	}
}

func TestRename_DetachedWorktree(t *testing.T) {
	repo := setupRenameTestRepo(t, "renamedetached")
	if err := runApp("new", "scratch", "--detach", "--ref", "HEAD", "--no-fetch"); err != nil {
		t.Fatalf("new detached failed: %v", err)
	}

	if err := runApp("rename", "scratch", "scratch-new"); err != nil {
		t.Fatalf("rename detached failed: %v", err)
	}

	repoGit := &git.Git{Dir: repo.BareDir}
	if out, _ := repoGit.Run("branch", "--list", "scratch-new"); out != "" {
		t.Fatalf("detached rename should not create branch: %s", out)
	}
	wts, err := worktree.List(repoGit)
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	wt, err := findWorktree(filterBareWorktrees(wts), "scratch-new")
	if err != nil {
		t.Fatalf("renamed detached worktree missing: %v", err)
	}
	if !wt.Detached {
		t.Fatalf("renamed worktree should remain detached")
	}
}

func TestRename_UpdatesStackMetadata(t *testing.T) {
	repo := setupRenameTestRepo(t, "renamestack")
	if err := runApp("new", "feature-a", "--no-fetch"); err != nil {
		t.Fatalf("new feature-a failed: %v", err)
	}
	if err := runApp("new", "feature-b", "--base", "feature-a", "--no-fetch"); err != nil {
		t.Fatalf("new feature-b failed: %v", err)
	}

	if err := runApp("rename", "feature-a", "feature-renamed"); err != nil {
		t.Fatalf("rename stack parent failed: %v", err)
	}

	st := stack.Load(repo.BareDir)
	if st.IsTracked("feature-a") {
		t.Fatal("old stack branch should not remain tracked")
	}
	if st.Parent("feature-renamed") == "" {
		t.Fatal("renamed stack branch should remain tracked")
	}
	if got := st.Parent("feature-b"); got != "feature-renamed" {
		t.Fatalf("feature-b parent = %q, want feature-renamed", got)
	}
}

func TestRename_MovesStatusDirAndRejectsCollision(t *testing.T) {
	repo := setupRenameTestRepo(t, "renamestatus")
	if err := runApp("new", "old", "--no-fetch"); err != nil {
		t.Fatalf("new old failed: %v", err)
	}
	writeActiveSessionFile(t, repo.RepoName, "old", "s1", claude.StatusDone)

	if err := runApp("rename", "old", "new"); err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(claude.StatusWorktreeDir(repo.RepoName, "new"), "s1.json")); err != nil {
		t.Fatalf("new status file missing: %v", err)
	}
	if _, err := os.Stat(claude.StatusWorktreeDir(repo.RepoName, "old")); !os.IsNotExist(err) {
		t.Fatalf("old status dir should be gone, err=%v", err)
	}

	if err := runApp("new", "old2", "--no-fetch"); err != nil {
		t.Fatalf("new old2 failed: %v", err)
	}
	if err := os.MkdirAll(claude.StatusWorktreeDir(repo.RepoName, "new2"), 0o755); err != nil {
		t.Fatalf("mkdir status collision: %v", err)
	}
	if err := runApp("rename", "old2", "new2"); err == nil {
		t.Fatal("rename should fail on status dir collision")
	}
	if _, err := os.Stat(filepath.Join(repo.WorktreeDir, "old2")); err != nil {
		t.Fatalf("old2 path should remain after collision: %v", err)
	}
	if out, _ := (&git.Git{Dir: repo.BareDir}).Run("branch", "--list", "old2"); out == "" {
		t.Fatal("old2 branch should remain after collision")
	}
}

func TestRename_RenamesTmuxSession(t *testing.T) {
	binDir, _ := installTestCLIPath(t, "")
	logPath := filepath.Join(binDir, "tmux.log")
	installFakeTmuxForRename(t, binDir, logPath, "renametmux/old", "")

	setupRenameTestRepo(t, "renametmux")
	if err := runApp("new", "old", "--no-fetch"); err != nil {
		t.Fatalf("new old failed: %v", err)
	}

	if err := runApp("rename", "old", "new"); err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read tmux log: %v", err)
	}
	if !strings.Contains(string(data), "rename-session|-t|renametmux/old|renametmux/new") {
		t.Fatalf("tmux rename not recorded:\n%s", data)
	}
}

func TestRename_RejectsTmuxSessionCollisionBeforeMutation(t *testing.T) {
	binDir, _ := installTestCLIPath(t, "")
	logPath := filepath.Join(binDir, "tmux.log")
	installFakeTmuxForRename(t, binDir, logPath, "", "renametmuxcollision/new")

	repo := setupRenameTestRepo(t, "renametmuxcollision")
	if err := runApp("new", "old", "--no-fetch"); err != nil {
		t.Fatalf("new old failed: %v", err)
	}

	if err := runApp("rename", "old", "new"); err == nil {
		t.Fatal("rename should fail on tmux session collision")
	}
	if _, err := os.Stat(filepath.Join(repo.WorktreeDir, "old")); err != nil {
		t.Fatalf("old path should remain after collision: %v", err)
	}
	if out, _ := (&git.Git{Dir: repo.BareDir}).Run("branch", "--list", "old"); out == "" {
		t.Fatal("old branch should remain after collision")
	}
}

func TestRename_RetargetsUpstreamAndWarnsWhenRemoteLeftBehind(t *testing.T) {
	repo := setupRenameTestRepo(t, "renameupstream")
	if err := runApp("new", "old", "--no-fetch"); err != nil {
		t.Fatalf("new old failed: %v", err)
	}
	oldPath := filepath.Join(repo.WorktreeDir, "old")
	commitFile(t, oldPath, "old.txt", "old\n", "old commit")
	if _, err := (&git.Git{Dir: oldPath}).Run("push", "-u", "origin", "old"); err != nil {
		t.Fatalf("push old: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runApp("rename", "old", "new")
	})
	if err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	if !strings.Contains(out, "Remote branch origin/old was left unchanged") {
		t.Fatalf("expected remote left-behind warning, got:\n%s", out)
	}

	repoGit := &git.Git{Dir: repo.BareDir}
	merge, err := repoGit.Run("config", "--get", "branch.new.merge")
	if err != nil {
		t.Fatalf("read branch.new.merge: %v", err)
	}
	if merge != "refs/heads/new" {
		t.Fatalf("branch.new.merge = %q, want refs/heads/new", merge)
	}
	if !remoteBranchExists(repoGit, "origin", "old") {
		t.Fatal("origin/old should remain after local-only rename")
	}
}

func TestRenameRemote_PushesNewAndDeletesOld(t *testing.T) {
	repo := setupRenameTestRepo(t, "renameremote")
	if err := runApp("new", "old", "--no-fetch"); err != nil {
		t.Fatalf("new old failed: %v", err)
	}
	oldPath := filepath.Join(repo.WorktreeDir, "old")
	commitFile(t, oldPath, "old.txt", "old\n", "old commit")
	if _, err := (&git.Git{Dir: oldPath}).Run("push", "-u", "origin", "old"); err != nil {
		t.Fatalf("push old: %v", err)
	}

	if err := runApp("rename", "old", "new", "--remote"); err != nil {
		t.Fatalf("rename --remote failed: %v", err)
	}

	repoGit := &git.Git{Dir: repo.BareDir}
	if remoteBranchExists(repoGit, "origin", "old") {
		t.Fatal("origin/old should be deleted")
	}
	if !remoteBranchExists(repoGit, "origin", "new") {
		t.Fatal("origin/new should exist")
	}
	merge, err := repoGit.Run("config", "--get", "branch.new.merge")
	if err != nil {
		t.Fatalf("read branch.new.merge: %v", err)
	}
	if merge != "refs/heads/new" {
		t.Fatalf("branch.new.merge = %q, want refs/heads/new", merge)
	}
}

func TestRenameRemote_RejectsExistingDestinationRemoteBranch(t *testing.T) {
	repo := setupRenameTestRepo(t, "renameremotedest")
	if err := runApp("new", "old", "--no-fetch"); err != nil {
		t.Fatalf("new old failed: %v", err)
	}
	oldPath := filepath.Join(repo.WorktreeDir, "old")
	commitFile(t, oldPath, "old.txt", "old\n", "old commit")
	if _, err := (&git.Git{Dir: oldPath}).Run("push", "-u", "origin", "old"); err != nil {
		t.Fatalf("push old: %v", err)
	}
	if _, err := (&git.Git{Dir: repo.MainDir}).Run("push", "origin", "HEAD:refs/heads/new"); err != nil {
		t.Fatalf("push destination branch: %v", err)
	}

	if err := runApp("rename", "old", "new", "--remote"); err == nil {
		t.Fatal("rename --remote should reject existing destination remote branch")
	}
	if _, err := os.Stat(filepath.Join(repo.WorktreeDir, "old")); err != nil {
		t.Fatalf("old path should remain after destination collision: %v", err)
	}
	if out, _ := (&git.Git{Dir: repo.BareDir}).Run("branch", "--list", "old"); out == "" {
		t.Fatal("old branch should remain after destination collision")
	}
	if out, _ := (&git.Git{Dir: repo.BareDir}).Run("branch", "--list", "new"); out != "" {
		t.Fatal("new branch should not exist after destination collision")
	}
}

func TestRenameRemote_RefusesRemoteOnlyCommits(t *testing.T) {
	repo := setupRenameTestRepo(t, "renameremotesafe")
	if err := runApp("new", "old", "--no-fetch"); err != nil {
		t.Fatalf("new old failed: %v", err)
	}
	oldPath := filepath.Join(repo.WorktreeDir, "old")
	commitFile(t, oldPath, "old.txt", "old\n", "old commit")
	if _, err := (&git.Git{Dir: oldPath}).Run("push", "-u", "origin", "old"); err != nil {
		t.Fatalf("push old: %v", err)
	}

	other := filepath.Join(repo.Home, "other")
	if _, err := (&git.Git{}).Run("clone", repo.Origin, other); err != nil {
		t.Fatalf("clone other: %v", err)
	}
	otherGit := &git.Git{Dir: other}
	if _, err := otherGit.Run("checkout", "old"); err != nil {
		t.Fatalf("checkout old in other clone: %v", err)
	}
	commitFile(t, other, "remote-only.txt", "remote\n", "remote only")
	if _, err := otherGit.Run("push", "origin", "old"); err != nil {
		t.Fatalf("push remote-only commit: %v", err)
	}

	if err := runApp("rename", "old", "new", "--remote"); err == nil {
		t.Fatal("rename --remote should refuse remote-only commits")
	}
	if _, err := os.Stat(filepath.Join(repo.WorktreeDir, "old")); err != nil {
		t.Fatalf("old path should remain after refused remote rename: %v", err)
	}
	if out, _ := (&git.Git{Dir: repo.BareDir}).Run("branch", "--list", "old"); out == "" {
		t.Fatal("old branch should remain after refused remote rename")
	}
	if out, _ := (&git.Git{Dir: repo.BareDir}).Run("branch", "--list", "new"); out != "" {
		t.Fatal("new branch should not exist after refused remote rename")
	}
}

func installFakeTmuxForRename(t *testing.T, binDir, logPath, oldSession, newSession string) {
	t.Helper()

	script := fmt.Sprintf(`#!/bin/sh
set -eu
if [ "$1" = "has-session" ]; then
  if [ "$3" = %q ] || [ "$3" = %q ]; then
    exit 0
  fi
  exit 1
fi
if [ "$1" = "rename-session" ]; then
  printf '%%s|%%s|%%s|%%s\n' "$1" "$2" "$3" "$4" >> %q
  exit 0
fi
exit 1
`, oldSession, newSession, logPath)
	if err := os.WriteFile(filepath.Join(binDir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatalf("write tmux stub: %v", err)
	}
}
