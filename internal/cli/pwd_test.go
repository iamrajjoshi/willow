package cli

import (
	"strings"
	"testing"

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

func TestPickWorktree_EmptyList(t *testing.T) {
	err := pickWorktree(nil)
	if err == nil {
		t.Fatal("expected error for empty worktree list")
	}
	if !strings.Contains(err.Error(), "no worktrees found") {
		t.Errorf("error = %q, want it to contain 'no worktrees found'", err.Error())
	}
}
