package cli

import (
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/worktree"
)

func TestExtractPathFromLine(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"🤖 BUSY  main  /home/user/.willow/worktrees/repo/main", "/home/user/.willow/worktrees/repo/main"},
		{"✅ DONE  feature/auth  /wt/repo/feature-auth", "/wt/repo/feature-auth"},
		{"🟡 IDLE  repo/branch  /some/path", "/some/path"},
		{"", ""},
		{"single-field", "single-field"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := extractPathFromLine(tt.line)
			if got != tt.want {
				t.Errorf("extractPathFromLine(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestStatusOrder(t *testing.T) {
	tests := []struct {
		status claude.Status
		want   int
	}{
		{claude.StatusBusy, 0},
		{claude.StatusWait, 1},
		{claude.StatusDone, 2},
		{claude.StatusIdle, 3},
		{claude.StatusOffline, 4},
		{claude.Status("UNKNOWN"), 4},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := statusOrder(tt.status)
			if got != tt.want {
				t.Errorf("statusOrder(%q) = %d, want %d", tt.status, got, tt.want)
			}
		})
	}

	// Verify ordering: BUSY < WAIT < DONE < IDLE < OFFLINE
	if statusOrder(claude.StatusBusy) >= statusOrder(claude.StatusWait) {
		t.Error("BUSY should sort before WAIT")
	}
	if statusOrder(claude.StatusWait) >= statusOrder(claude.StatusDone) {
		t.Error("WAIT should sort before DONE")
	}
	if statusOrder(claude.StatusDone) >= statusOrder(claude.StatusIdle) {
		t.Error("DONE should sort before IDLE")
	}
}

func TestBuildWorktreeLines(t *testing.T) {
	// Use a temp HOME so claude.ReadStatus returns offline for all
	home := t.TempDir()
	t.Setenv("HOME", home)

	wts := []worktree.Worktree{
		{Branch: "main", Path: "/wt/repo/main"},
		{Branch: "feature/auth", Path: "/wt/repo/feature-auth"},
	}

	lines := buildWorktreeLines(wts, "repo")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	for _, line := range lines {
		// Each line should contain a path
		if !strings.Contains(line, "/wt/repo/") {
			t.Errorf("line should contain path, got %q", line)
		}
	}
}

func TestBuildCrossRepoWorktreeLines(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rwts := []repoWorktree{
		{
			Repo:     repoInfo{Name: "alpha", BareDir: "/repos/alpha.git"},
			Worktree: worktree.Worktree{Branch: "main", Path: "/wt/alpha/main"},
		},
		{
			Repo:     repoInfo{Name: "beta", BareDir: "/repos/beta.git"},
			Worktree: worktree.Worktree{Branch: "feature", Path: "/wt/beta/feature"},
		},
	}

	lines := buildCrossRepoWorktreeLines(rwts)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Lines should contain repo/branch format
	foundAlpha := false
	foundBeta := false
	for _, line := range lines {
		if strings.Contains(line, "alpha/main") {
			foundAlpha = true
		}
		if strings.Contains(line, "beta/feature") {
			foundBeta = true
		}
		// Path should be the last field
		path := extractPathFromLine(line)
		if !strings.HasPrefix(path, "/wt/") {
			t.Errorf("expected path to start with /wt/, got %q", path)
		}
	}
	if !foundAlpha {
		t.Error("expected alpha/main in lines")
	}
	if !foundBeta {
		t.Error("expected beta/feature in lines")
	}
}
