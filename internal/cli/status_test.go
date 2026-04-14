package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/worktree"
)

func TestCollectRepoStatus_NoSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	wts := []worktree.Worktree{
		{Branch: "main", Path: "/wt/repo/main"},
		{Branch: "feature", Path: "/wt/repo/feature"},
	}

	rs := collectRepoStatus("repo", wts, nil)
	if rs.WorktreeCount != 2 {
		t.Errorf("WorktreeCount = %d, want 2", rs.WorktreeCount)
	}
	if rs.ActiveCount != 0 {
		t.Errorf("ActiveCount = %d, want 0", rs.ActiveCount)
	}
	if rs.UnreadCount != 0 {
		t.Errorf("UnreadCount = %d, want 0", rs.UnreadCount)
	}
	if len(rs.Entries) != 2 {
		t.Errorf("Entries = %d, want 2", len(rs.Entries))
	}
	for _, e := range rs.Entries {
		if e.Status != string(claude.StatusOffline) {
			t.Errorf("Entry status = %q, want %q", e.Status, claude.StatusOffline)
		}
	}
}

func TestCollectRepoStatus_WithSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "feature"

	// Create session files
	sessDir := filepath.Join(home, ".willow", "status", repoName, wtName)
	os.MkdirAll(sessDir, 0o755)

	now := time.Now().UTC()
	sessions := []claude.SessionStatus{
		{Status: claude.StatusBusy, SessionID: "s1", Timestamp: now},
		{Status: claude.StatusDone, SessionID: "s2", Timestamp: now},
	}
	for _, ss := range sessions {
		data, _ := json.Marshal(ss)
		os.WriteFile(filepath.Join(sessDir, ss.SessionID+".json"), data, 0o644)
	}

	wts := []worktree.Worktree{
		{Branch: "feature", Path: "/wt/myrepo/" + wtName},
	}

	rs := collectRepoStatus(repoName, wts, nil)
	if rs.ActiveCount != 2 {
		t.Errorf("ActiveCount = %d, want 2", rs.ActiveCount)
	}
	if rs.UnreadCount != 1 {
		t.Errorf("UnreadCount = %d, want 1 (DONE session is unread)", rs.UnreadCount)
	}
	if len(rs.Entries) != 2 {
		t.Errorf("Entries = %d, want 2 (one per session)", len(rs.Entries))
	}
}

func TestCollectRepoStatus_RepoNameSet(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	wts := []worktree.Worktree{
		{Branch: "main", Path: "/wt/myrepo/main"},
	}

	rs := collectRepoStatus("myrepo", wts, nil)
	if rs.Name != "myrepo" {
		t.Errorf("Name = %q, want %q", rs.Name, "myrepo")
	}
	if len(rs.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(rs.Entries))
	}
	if rs.Entries[0].Repo != "myrepo" {
		t.Errorf("Entry.Repo = %q, want %q", rs.Entries[0].Repo, "myrepo")
	}
	if rs.Entries[0].Branch != "main" {
		t.Errorf("Entry.Branch = %q, want %q", rs.Entries[0].Branch, "main")
	}
}

func TestCollectRepoStatus_UnreadMarking(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "done-wt"
	sessDir := filepath.Join(home, ".willow", "status", repoName, wtName)
	os.MkdirAll(sessDir, 0o755)

	// Use a past timestamp so MarkRead (which writes "now") is strictly after it
	past := time.Now().UTC().Add(-5 * time.Second)
	ss := claude.SessionStatus{Status: claude.StatusDone, SessionID: "s1", Timestamp: past}
	data, _ := json.Marshal(ss)
	os.WriteFile(filepath.Join(sessDir, "s1.json"), data, 0o644)

	wts := []worktree.Worktree{
		{Branch: "done-branch", Path: "/wt/myrepo/" + wtName},
	}

	rs := collectRepoStatus(repoName, wts, nil)
	if len(rs.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(rs.Entries))
	}
	if !rs.Entries[0].Unread {
		t.Error("DONE session should be marked unread")
	}

	// Mark as read, then re-collect
	claude.MarkRead(repoName, wtName)
	rs = collectRepoStatus(repoName, wts, nil)
	if rs.Entries[0].Unread {
		t.Error("DONE session should not be unread after MarkRead")
	}
	if rs.UnreadCount != 0 {
		t.Errorf("UnreadCount = %d, want 0 after MarkRead", rs.UnreadCount)
	}
}

func TestCollectRepoStatus_EmptyWorktrees(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rs := collectRepoStatus("empty", nil, nil)
	if rs.WorktreeCount != 0 {
		t.Errorf("WorktreeCount = %d, want 0", rs.WorktreeCount)
	}
	if len(rs.Entries) != 0 {
		t.Errorf("Entries = %d, want 0", len(rs.Entries))
	}
}
