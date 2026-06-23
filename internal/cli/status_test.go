package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iamrajjoshi/willow/internal/agent"
	"github.com/iamrajjoshi/willow/internal/termfmt"
	"github.com/iamrajjoshi/willow/internal/ui"
	"github.com/iamrajjoshi/willow/internal/worktree"
)

func TestCollectRepoStatus_NoSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	wts := []worktree.Worktree{
		{Branch: "main", Path: "/wt/repo/main"},
		{Branch: "feature", Path: "/wt/repo/feature"},
	}

	rs := collectRepoStatus("repo", wts)
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
		if e.Status != string(agent.StatusOffline) {
			t.Errorf("Entry status = %q, want %q", e.Status, agent.StatusOffline)
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
	sessions := []agent.SessionStatus{
		{Status: agent.StatusBusy, SessionID: "s1", Timestamp: now},
		{Status: agent.StatusDone, SessionID: "s2", Timestamp: now},
	}
	for _, ss := range sessions {
		data, _ := json.Marshal(ss)
		os.WriteFile(filepath.Join(sessDir, ss.SessionID+".json"), data, 0o644)
	}

	wts := []worktree.Worktree{
		{Branch: "feature", Path: "/wt/myrepo/" + wtName},
	}

	rs := collectRepoStatus(repoName, wts)
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

	rs := collectRepoStatus("myrepo", wts)
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

func TestStatusBranchLabel(t *testing.T) {
	if got := statusBranchLabel("feature", "claude", "1234567890"); got != "feature [claude:12345678]" {
		t.Errorf("statusBranchLabel with session = %q, want %q", got, "feature [claude:12345678]")
	}
	if got := statusBranchLabel("main", "", ""); got != "main" {
		t.Errorf("statusBranchLabel without session = %q, want %q", got, "main")
	}
}

func TestFormatStatusEntryLinesFitsNarrowWidth(t *testing.T) {
	u := &ui.UI{}
	entries := []sessionEntry{
		{
			Branch:    "raj--tprm-464--backend-validate-review-risk-subtype",
			Harness:   "codex",
			SessionID: "1234567890abcdef",
			Status:    string(agent.StatusDone),
			Timestamp: "2h ago",
			Unread:    true,
		},
	}
	lines := formatStatusEntryLines(u, entries, 52)
	for _, line := range lines {
		if got := termfmt.VisibleWidth(line); got > 52 {
			t.Fatalf("line width = %d, want <= 52:\n%s", got, termfmt.StripANSI(line))
		}
	}
	plain := termfmt.StripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(plain, "DONE\u25CF") || !strings.Contains(plain, "2h ago") {
		t.Fatalf("status line should preserve status and timestamp:\n%s", plain)
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
	ss := agent.SessionStatus{Status: agent.StatusDone, SessionID: "s1", Timestamp: past}
	data, _ := json.Marshal(ss)
	os.WriteFile(filepath.Join(sessDir, "s1.json"), data, 0o644)

	wts := []worktree.Worktree{
		{Branch: "done-branch", Path: "/wt/myrepo/" + wtName},
	}

	rs := collectRepoStatus(repoName, wts)
	if len(rs.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(rs.Entries))
	}
	if !rs.Entries[0].Unread {
		t.Error("DONE session should be marked unread")
	}

	// Mark as read, then re-collect
	agent.MarkRead(repoName, wtName)
	rs = collectRepoStatus(repoName, wts)
	if rs.Entries[0].Unread {
		t.Error("DONE session should not be unread after MarkRead")
	}
	if rs.UnreadCount != 0 {
		t.Errorf("UnreadCount = %d, want 0 after MarkRead", rs.UnreadCount)
	}
}

func TestCollectRepoStatus_MarksOnlyUnreadDoneSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "done-wt"
	sessDir := filepath.Join(home, ".willow", "status", repoName, wtName)
	os.MkdirAll(sessDir, 0o755)

	base := time.Now().UTC().Add(-10 * time.Minute)
	sessions := []agent.SessionStatus{
		{Status: agent.StatusDone, SessionID: "old", Timestamp: base},
		{Status: agent.StatusDone, SessionID: "new", Timestamp: base.Add(2 * time.Minute)},
	}
	for _, ss := range sessions {
		data, _ := json.Marshal(ss)
		os.WriteFile(filepath.Join(sessDir, ss.SessionID+".json"), data, 0o644)
	}
	lastRead := base.Add(1*time.Minute).Format(time.RFC3339) + "\n"
	os.WriteFile(filepath.Join(sessDir, ".lastread"), []byte(lastRead), 0o644)

	rs := collectRepoStatus(repoName, []worktree.Worktree{
		{Branch: "done-branch", Path: "/wt/myrepo/" + wtName},
	})
	if rs.UnreadCount != 1 {
		t.Fatalf("UnreadCount = %d, want 1", rs.UnreadCount)
	}

	unreadBySession := map[string]bool{}
	for _, e := range rs.Entries {
		unreadBySession[e.SessionID] = e.Unread
	}
	if unreadBySession["old"] {
		t.Fatal("old DONE session should not be marked unread")
	}
	if !unreadBySession["new"] {
		t.Fatal("new DONE session should be marked unread")
	}
}

func TestCollectRepoStatus_EmptyWorktrees(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rs := collectRepoStatus("empty", nil)
	if rs.WorktreeCount != 0 {
		t.Errorf("WorktreeCount = %d, want 0", rs.WorktreeCount)
	}
	if len(rs.Entries) != 0 {
		t.Errorf("Entries = %d, want 0", len(rs.Entries))
	}
}
