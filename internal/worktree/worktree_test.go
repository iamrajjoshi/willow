package worktree

import "testing"

func TestParsePorcelain_SingleWorktree(t *testing.T) {
	input := `worktree /home/user/.willow/repos/myrepo.git
HEAD abc123
bare
`
	wts := parsePorcelain(input)
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}
	if wts[0].Path != "/home/user/.willow/repos/myrepo.git" {
		t.Errorf("Path = %q", wts[0].Path)
	}
	if wts[0].Head != "abc123" {
		t.Errorf("Head = %q", wts[0].Head)
	}
	if !wts[0].IsBare {
		t.Error("IsBare should be true")
	}
	if wts[0].Branch != "" {
		t.Errorf("Branch = %q, want empty for bare", wts[0].Branch)
	}
}

func TestParsePorcelain_MultipleWorktrees(t *testing.T) {
	input := `worktree /repos/myrepo.git
HEAD aaa111
bare

worktree /wt/myrepo/main
HEAD bbb222
branch refs/heads/main

worktree /wt/myrepo/feature-auth
HEAD ccc333
branch refs/heads/feature/auth
`
	wts := parsePorcelain(input)
	if len(wts) != 3 {
		t.Fatalf("expected 3 worktrees, got %d", len(wts))
	}

	// Bare repo
	if !wts[0].IsBare {
		t.Error("wt[0] should be bare")
	}

	// Main worktree
	if wts[1].Branch != "main" {
		t.Errorf("wt[1].Branch = %q, want %q", wts[1].Branch, "main")
	}
	if wts[1].Path != "/wt/myrepo/main" {
		t.Errorf("wt[1].Path = %q", wts[1].Path)
	}

	// Feature worktree
	if wts[2].Branch != "feature/auth" {
		t.Errorf("wt[2].Branch = %q, want %q", wts[2].Branch, "feature/auth")
	}
}

func TestParsePorcelain_DetachedHead(t *testing.T) {
	input := `worktree /wt/myrepo/detached
HEAD ddd444
detached
`
	wts := parsePorcelain(input)
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}
	if wts[0].Branch != "(detached)" {
		t.Errorf("Branch = %q, want %q", wts[0].Branch, "(detached)")
	}
	if !wts[0].Detached {
		t.Error("Detached should be true")
	}
	if got := wts[0].DisplayName(); got != "detached [detached ddd444]" {
		t.Errorf("DisplayName() = %q", got)
	}
	if got := wts[0].MatchName(); got != "detached" {
		t.Errorf("MatchName() = %q", got)
	}
}

func TestParsePorcelain_EmptyInput(t *testing.T) {
	wts := parsePorcelain("")
	if len(wts) != 0 {
		t.Errorf("expected 0 worktrees, got %d", len(wts))
	}
}

func TestParsePorcelain_MissingPath(t *testing.T) {
	// A block with no "worktree " line should be skipped
	input := `HEAD abc123
branch refs/heads/main
`
	wts := parsePorcelain(input)
	if len(wts) != 0 {
		t.Errorf("expected 0 worktrees for block without path, got %d", len(wts))
	}
}
