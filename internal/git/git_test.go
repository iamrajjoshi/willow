package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupRemoteAndClone creates a bare "remote" repo with one commit on main
// and clones it into a working dir. Returns the working dir path.
func setupRemoteAndClone(t *testing.T, defaultBranch string) string {
	t.Helper()

	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run(root, "init", "--bare", "--initial-branch="+defaultBranch, remote)
	run(root, "init", "--initial-branch="+defaultBranch, work)
	run(work, "config", "user.email", "test@test")
	run(work, "config", "user.name", "test")
	run(work, "remote", "add", "origin", remote)

	if err := os.WriteFile(filepath.Join(work, "seed"), []byte("seed"), 0644); err != nil {
		t.Fatal(err)
	}
	run(work, "add", "seed")
	run(work, "commit", "-m", "seed")
	run(work, "push", "-u", "origin", defaultBranch)
	return work
}

func TestResolveBaseBranch(t *testing.T) {
	work := setupRemoteAndClone(t, "master")
	g := &Git{Dir: work}

	if got := g.ResolveBaseBranch("develop"); got != "develop" {
		t.Errorf("configured value ignored: got %q, want %q", got, "develop")
	}
	if got := g.ResolveBaseBranch(""); got != "master" {
		t.Errorf("fallback to default branch failed: got %q, want %q", got, "master")
	}

	// Invalid dir: no symbolic-ref, falls back to hardcoded "main".
	badGit := &Git{Dir: t.TempDir()}
	if got := badGit.ResolveBaseBranch(""); got != "main" {
		t.Errorf("fallback when no git repo: got %q, want %q", got, "main")
	}
}

func TestMergedBranches_ExcludesTrivialMerges(t *testing.T) {
	work := setupRemoteAndClone(t, "main")
	g := &Git{Dir: work}

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// "trivial" branch: freshly forked from origin/main with zero commits.
	// This is the case that was being falsely reported as merged.
	runGit("branch", "feature-trivial", "origin/main")

	// "unmerged" branch: has unique commits not in origin/main.
	runGit("checkout", "-b", "feature-unmerged", "origin/main")
	if err := os.WriteFile(filepath.Join(work, "b"), []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "b")
	runGit("commit", "-m", "unmerged work")

	merged, err := g.MergedBranches("main")
	if err != nil {
		t.Fatalf("MergedBranches: %v", err)
	}

	set := make(map[string]bool)
	for _, b := range merged {
		set[b] = true
	}
	if set["feature-trivial"] {
		t.Errorf("trivially-forked branch should not appear as merged, got %v", merged)
	}
	if set["feature-unmerged"] {
		t.Errorf("unmerged branch should not appear as merged, got %v", merged)
	}
}

func TestMergedBranches_IncludesRealMerges(t *testing.T) {
	work := setupRemoteAndClone(t, "main")
	g := &Git{Dir: work}

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Real merge: branch with commits merged back into main.
	runGit("checkout", "-b", "feature-real", "origin/main")
	if err := os.WriteFile(filepath.Join(work, "a"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "a")
	runGit("commit", "-m", "feature work")
	runGit("checkout", "main")
	runGit("merge", "--no-ff", "feature-real", "-m", "merge")
	runGit("push", "origin", "main")
	runGit("fetch", "origin")

	merged, err := g.MergedBranches("main")
	if err != nil {
		t.Fatalf("MergedBranches: %v", err)
	}
	var found bool
	for _, b := range merged {
		if b == "feature-real" {
			found = true
		}
	}
	if !found {
		t.Errorf("real merged branch missing from %v", merged)
	}
}

func TestMergedBranchSet_FiltersToGivenBranches(t *testing.T) {
	work := setupRemoteAndClone(t, "main")
	g := &Git{Dir: work}

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	runGit("checkout", "-b", "wt-merged", "origin/main")
	if err := os.WriteFile(filepath.Join(work, "a"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "a")
	runGit("commit", "-m", "wt-merged work")
	runGit("checkout", "main")
	runGit("merge", "--no-ff", "wt-merged", "-m", "merge wt-merged")
	runGit("push", "origin", "main")
	runGit("fetch", "origin")

	runGit("checkout", "-b", "wt-unmerged", "origin/main")
	if err := os.WriteFile(filepath.Join(work, "b"), []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "b")
	runGit("commit", "-m", "wt-unmerged work")

	runGit("checkout", "-b", "other-merged", "origin/main")
	if err := os.WriteFile(filepath.Join(work, "c"), []byte("c"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "c")
	runGit("commit", "-m", "other work")
	runGit("checkout", "main")
	runGit("merge", "--no-ff", "other-merged", "-m", "merge other")
	runGit("push", "origin", "main")
	runGit("fetch", "origin")

	set := g.MergedBranchSet("main", []string{"wt-merged", "wt-unmerged"})
	if !set["wt-merged"] {
		t.Errorf("wt-merged should be in set, got %v", set)
	}
	if set["wt-unmerged"] {
		t.Errorf("wt-unmerged should not be in set, got %v", set)
	}
	if set["other-merged"] {
		t.Errorf("other-merged not in input list, must not leak into set, got %v", set)
	}
}

func TestMergedBranchSet_ExcludesTrivialBranchesAtBaseTip(t *testing.T) {
	work := setupRemoteAndClone(t, "main")
	g := &Git{Dir: work}

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	runGit("branch", "wt-trivial", "origin/main")

	set := g.MergedBranchSet("main", []string{"wt-trivial"})
	if set["wt-trivial"] {
		t.Errorf("wt-trivial should be excluded as a zero-commit branch, got %v", set)
	}
}

func TestMergedBranchSet_EmptyInput(t *testing.T) {
	work := setupRemoteAndClone(t, "main")
	g := &Git{Dir: work}
	set := g.MergedBranchSet("main", nil)
	if len(set) != 0 {
		t.Errorf("empty input should return empty set, got %v", set)
	}
}

func TestRemoteBranchesFiltersOriginHead(t *testing.T) {
	work := setupRemoteAndClone(t, "main")
	g := &Git{Dir: work}

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	runGit("checkout", "-b", "remote-feature", "origin/main")
	if err := os.WriteFile(filepath.Join(work, "remote-feature.txt"), []byte("remote feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "remote-feature.txt")
	runGit("commit", "-m", "remote feature")
	runGit("push", "origin", "remote-feature")
	runGit("fetch", "origin")
	runGit("remote", "set-head", "origin", "main")

	branches, err := g.RemoteBranches()
	if err != nil {
		t.Fatalf("RemoteBranches() error = %v", err)
	}
	set := make(map[string]bool, len(branches))
	for _, branch := range branches {
		set[branch] = true
		if branch == "HEAD" || branch == "origin/HEAD" {
			t.Fatalf("RemoteBranches() should filter HEAD pointers, got %v", branches)
		}
		if strings.HasPrefix(branch, "origin/") {
			t.Fatalf("RemoteBranches() should trim origin/ prefix, got %v", branches)
		}
	}
	for _, want := range []string{"main", "remote-feature"} {
		if !set[want] {
			t.Fatalf("RemoteBranches() = %v, missing %q", branches, want)
		}
	}
}

func TestDirtyAheadUnpushedAndRebaseHelpers(t *testing.T) {
	work := setupRemoteAndClone(t, "main")
	g := &Git{Dir: work}

	dirty, err := g.IsDirty()
	if err != nil {
		t.Fatalf("IsDirty() error = %v", err)
	}
	if dirty {
		t.Fatal("fresh checkout should be clean")
	}

	if err := os.WriteFile(filepath.Join(work, "change.txt"), []byte("change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err = g.IsDirty()
	if err != nil {
		t.Fatalf("IsDirty() after write error = %v", err)
	}
	if !dirty {
		t.Fatal("untracked file should mark worktree dirty")
	}

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("add", "change.txt")
	run("commit", "-m", "change")

	dirty, err = g.IsDirty()
	if err != nil {
		t.Fatalf("IsDirty() after commit error = %v", err)
	}
	if dirty {
		t.Fatal("committed worktree should be clean")
	}

	ahead, err := g.CommitsAhead("origin/main")
	if err != nil {
		t.Fatalf("CommitsAhead() error = %v", err)
	}
	if ahead != 1 {
		t.Fatalf("CommitsAhead(origin/main) = %d, want 1", ahead)
	}

	unpushed, err := g.HasUnpushedCommits()
	if err != nil {
		t.Fatalf("HasUnpushedCommits() error = %v", err)
	}
	if !unpushed {
		t.Fatal("local commit should be unpushed")
	}
	run("push", "origin", "main")
	unpushed, err = g.HasUnpushedCommits()
	if err != nil {
		t.Fatalf("HasUnpushedCommits() after push error = %v", err)
	}
	if unpushed {
		t.Fatal("pushed branch should not have unpushed commits")
	}

	gitDir, err := g.Run("rev-parse", "--git-dir")
	if err != nil {
		t.Fatalf("rev-parse --git-dir: %v", err)
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(work, gitDir)
	}
	rebaseDir := filepath.Join(gitDir, "rebase-merge")
	if err := os.MkdirAll(rebaseDir, 0o755); err != nil {
		t.Fatalf("mkdir rebase dir: %v", err)
	}
	if !g.IsRebaseInProgress() {
		t.Fatal("synthetic rebase-merge dir should be detected")
	}
	if err := os.RemoveAll(rebaseDir); err != nil {
		t.Fatalf("remove rebase dir: %v", err)
	}
	if g.IsRebaseInProgress() {
		t.Fatal("removed rebase dir should not be detected")
	}
}
