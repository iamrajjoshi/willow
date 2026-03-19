package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/worktree"
)

var testWorktrees = []worktree.Worktree{
	{Branch: "main", Path: "/home/user/.willow/worktrees/repo/main"},
	{Branch: "feature/auth", Path: "/home/user/.willow/worktrees/repo/feature-auth"},
	{Branch: "feature/payments", Path: "/home/user/.willow/worktrees/repo/feature-payments"},
	{Branch: "alice/bugfix", Path: "/home/user/.willow/worktrees/repo/alice-bugfix"},
}

func TestFindWorktree_ExactBranch(t *testing.T) {
	wt, err := findWorktree(testWorktrees, "feature/auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wt.Branch != "feature/auth" {
		t.Errorf("Branch = %q, want %q", wt.Branch, "feature/auth")
	}
}

func TestFindWorktree_SubstringBranch(t *testing.T) {
	wt, err := findWorktree(testWorktrees, "bugfix")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wt.Branch != "alice/bugfix" {
		t.Errorf("Branch = %q, want %q", wt.Branch, "alice/bugfix")
	}
}

func TestFindWorktree_DirectorySuffix(t *testing.T) {
	wt, err := findWorktree(testWorktrees, "feature-auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wt.Branch != "feature/auth" {
		t.Errorf("Branch = %q, want %q", wt.Branch, "feature/auth")
	}
}

func TestFindWorktree_Ambiguous(t *testing.T) {
	_, err := findWorktree(testWorktrees, "feature")
	if err == nil {
		t.Fatal("expected error for ambiguous match")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want it to contain 'ambiguous'", err.Error())
	}
}

func TestFindWorktree_NotFound(t *testing.T) {
	_, err := findWorktree(testWorktrees, "nonexistent")
	if err == nil {
		t.Fatal("expected error for no match")
	}
	if !strings.Contains(err.Error(), "no worktree found") {
		t.Errorf("error = %q, want it to contain 'no worktree found'", err.Error())
	}
}

func TestFindWorktree_ExactMatchTakesPriority(t *testing.T) {
	wt, err := findWorktree(testWorktrees, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wt.Branch != "main" {
		t.Errorf("Branch = %q, want %q (exact match should win)", wt.Branch, "main")
	}
}

func TestFilterBareWorktrees(t *testing.T) {
	worktrees := []worktree.Worktree{
		{Branch: "main", Path: "/path/main", IsBare: false},
		{Branch: "", Path: "/path/bare", IsBare: true},
		{Branch: "feature", Path: "/path/feature", IsBare: false},
	}
	filtered := filterBareWorktrees(worktrees)
	if len(filtered) != 2 {
		t.Errorf("expected 2 non-bare worktrees, got %d", len(filtered))
	}
}

// --- Cross-repo helper tests ---

var testCrossRepoWorktrees = []repoWorktree{
	{
		Repo:     repoInfo{Name: "alpha", BareDir: "/repos/alpha.git"},
		Worktree: worktree.Worktree{Branch: "main", Path: "/wt/alpha/main"},
	},
	{
		Repo:     repoInfo{Name: "alpha", BareDir: "/repos/alpha.git"},
		Worktree: worktree.Worktree{Branch: "feature/auth", Path: "/wt/alpha/feature-auth"},
	},
	{
		Repo:     repoInfo{Name: "beta", BareDir: "/repos/beta.git"},
		Worktree: worktree.Worktree{Branch: "main", Path: "/wt/beta/main"},
	},
	{
		Repo:     repoInfo{Name: "beta", BareDir: "/repos/beta.git"},
		Worktree: worktree.Worktree{Branch: "feature/payments", Path: "/wt/beta/feature-payments"},
	},
}

func TestFindCrossRepoWorktree_ExactBranch(t *testing.T) {
	rwt, err := findCrossRepoWorktree(testCrossRepoWorktrees, "feature/auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rwt.Worktree.Branch != "feature/auth" {
		t.Errorf("Branch = %q, want %q", rwt.Worktree.Branch, "feature/auth")
	}
	if rwt.Repo.Name != "alpha" {
		t.Errorf("Repo = %q, want %q", rwt.Repo.Name, "alpha")
	}
}

func TestFindCrossRepoWorktree_Substring(t *testing.T) {
	rwt, err := findCrossRepoWorktree(testCrossRepoWorktrees, "payments")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rwt.Worktree.Branch != "feature/payments" {
		t.Errorf("Branch = %q, want %q", rwt.Worktree.Branch, "feature/payments")
	}
	if rwt.Repo.Name != "beta" {
		t.Errorf("Repo = %q, want %q", rwt.Repo.Name, "beta")
	}
}

func TestFindCrossRepoWorktree_DirectorySuffix(t *testing.T) {
	rwt, err := findCrossRepoWorktree(testCrossRepoWorktrees, "feature-payments")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rwt.Worktree.Branch != "feature/payments" {
		t.Errorf("Branch = %q, want %q", rwt.Worktree.Branch, "feature/payments")
	}
}

func TestFindCrossRepoWorktree_AmbiguousAcrossRepos(t *testing.T) {
	_, err := findCrossRepoWorktree(testCrossRepoWorktrees, "feature")
	if err == nil {
		t.Fatal("expected error for ambiguous match across repos")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want it to contain 'ambiguous'", err.Error())
	}
	// Should include repo/branch format in the error
	if !strings.Contains(err.Error(), "alpha/feature/auth") {
		t.Errorf("error should mention alpha/feature/auth, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "beta/feature/payments") {
		t.Errorf("error should mention beta/feature/payments, got: %s", err.Error())
	}
}

func TestFindCrossRepoWorktree_NotFound(t *testing.T) {
	_, err := findCrossRepoWorktree(testCrossRepoWorktrees, "nonexistent")
	if err == nil {
		t.Fatal("expected error for no match")
	}
	if !strings.Contains(err.Error(), "no worktree found") {
		t.Errorf("error = %q, want it to contain 'no worktree found'", err.Error())
	}
}

func TestFindCrossRepoWorktree_ExactMatchPriority(t *testing.T) {
	// "main" exists in both repos; exact match returns the first one found
	rwt, err := findCrossRepoWorktree(testCrossRepoWorktrees, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rwt.Worktree.Branch != "main" {
		t.Errorf("Branch = %q, want %q", rwt.Worktree.Branch, "main")
	}
}

func TestRepoWorktreeByPath(t *testing.T) {
	rwt := repoWorktreeByPath(testCrossRepoWorktrees, "/wt/beta/feature-payments")
	if rwt == nil {
		t.Fatal("expected non-nil result")
	}
	if rwt.Repo.Name != "beta" {
		t.Errorf("Repo = %q, want %q", rwt.Repo.Name, "beta")
	}
	if rwt.Worktree.Branch != "feature/payments" {
		t.Errorf("Branch = %q, want %q", rwt.Worktree.Branch, "feature/payments")
	}
}

func TestRepoWorktreeByPath_NotFound(t *testing.T) {
	rwt := repoWorktreeByPath(testCrossRepoWorktrees, "/nonexistent")
	if rwt != nil {
		t.Error("expected nil for non-matching path")
	}
}

func TestResolveRepos_WithRepoFlag(t *testing.T) {
	origin := setupTestEnv(t)
	if err := runApp("clone", origin, "flagtest"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	g := &git.Git{}
	repos, err := resolveRepos(g, "flagtest")
	if err != nil {
		t.Fatalf("resolveRepos with flag: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != "flagtest" {
		t.Errorf("Name = %q, want %q", repos[0].Name, "flagtest")
	}
}

func TestResolveRepos_CurrentRepo(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "currepo"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	// cd into the worktree
	worktreeDir := filepath.Join(home, ".willow", "worktrees", "currepo")
	entries, _ := os.ReadDir(worktreeDir)
	os.Chdir(filepath.Join(worktreeDir, entries[0].Name()))

	g := &git.Git{}
	repos, err := resolveRepos(g, "")
	if err != nil {
		t.Fatalf("resolveRepos from worktree: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != "currepo" {
		t.Errorf("Name = %q, want %q", repos[0].Name, "currepo")
	}
}

func TestResolveRepos_AllRepos(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "repo1"); err != nil {
		t.Fatalf("clone repo1 failed: %v", err)
	}
	if err := runApp("clone", origin, "repo2"); err != nil {
		t.Fatalf("clone repo2 failed: %v", err)
	}

	// cd to home (outside any repo)
	os.Chdir(home)

	g := &git.Git{}
	repos, err := resolveRepos(g, "")
	if err != nil {
		t.Fatalf("resolveRepos fallback: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	names := map[string]bool{}
	for _, r := range repos {
		names[r.Name] = true
	}
	if !names["repo1"] || !names["repo2"] {
		t.Errorf("expected repo1 and repo2, got %v", names)
	}
}

func TestCollectAllWorktrees(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "col1"); err != nil {
		t.Fatalf("clone col1 failed: %v", err)
	}
	if err := runApp("clone", origin, "col2"); err != nil {
		t.Fatalf("clone col2 failed: %v", err)
	}

	// Create an extra worktree in col1
	worktreeDir := filepath.Join(home, ".willow", "worktrees", "col1")
	entries, _ := os.ReadDir(worktreeDir)
	os.Chdir(filepath.Join(worktreeDir, entries[0].Name()))
	if err := runApp("new", "extra-branch", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	repos := []repoInfo{
		{Name: "col1", BareDir: filepath.Join(home, ".willow", "repos", "col1.git")},
		{Name: "col2", BareDir: filepath.Join(home, ".willow", "repos", "col2.git")},
	}
	all := collectAllWorktrees(repos, false)
	// col1 has 2 worktrees (main + extra-branch), col2 has 1 (main)
	if len(all) != 3 {
		t.Errorf("expected 3 worktrees total, got %d", len(all))
	}

	// Verify repo names are set
	repoNames := map[string]int{}
	for _, rwt := range all {
		repoNames[rwt.Repo.Name]++
	}
	if repoNames["col1"] != 2 {
		t.Errorf("expected 2 worktrees from col1, got %d", repoNames["col1"])
	}
	if repoNames["col2"] != 1 {
		t.Errorf("expected 1 worktree from col2, got %d", repoNames["col2"])
	}
}
