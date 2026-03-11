package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUnreadCount_NoDoneSessionsReturnsZero(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "wt1"
	sessDir := filepath.Join(home, ".willow", "status", repoName, wtName)
	os.MkdirAll(sessDir, 0o755)

	ss := SessionStatus{Status: StatusBusy, SessionID: "s1", Timestamp: time.Now().UTC()}
	data, _ := json.Marshal(ss)
	os.WriteFile(filepath.Join(sessDir, "s1.json"), data, 0o644)

	if got := UnreadCount(repoName, wtName); got != 0 {
		t.Errorf("UnreadCount = %d, want 0", got)
	}
}

func TestUnreadCount_DoneSessionIsUnread(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "wt2"
	sessDir := filepath.Join(home, ".willow", "status", repoName, wtName)
	os.MkdirAll(sessDir, 0o755)

	ss := SessionStatus{Status: StatusDone, SessionID: "s1", Timestamp: time.Now().UTC()}
	data, _ := json.Marshal(ss)
	os.WriteFile(filepath.Join(sessDir, "s1.json"), data, 0o644)

	if got := UnreadCount(repoName, wtName); got != 1 {
		t.Errorf("UnreadCount = %d, want 1", got)
	}
	if !IsUnread(repoName, wtName) {
		t.Error("IsUnread = false, want true")
	}
}

func TestMarkRead_ClearsUnread(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "wt3"
	sessDir := filepath.Join(home, ".willow", "status", repoName, wtName)
	os.MkdirAll(sessDir, 0o755)

	past := time.Now().UTC().Add(-1 * time.Minute)
	ss := SessionStatus{Status: StatusDone, SessionID: "s1", Timestamp: past}
	data, _ := json.Marshal(ss)
	os.WriteFile(filepath.Join(sessDir, "s1.json"), data, 0o644)

	if err := MarkRead(repoName, wtName); err != nil {
		t.Fatalf("MarkRead error: %v", err)
	}

	if got := UnreadCount(repoName, wtName); got != 0 {
		t.Errorf("UnreadCount after MarkRead = %d, want 0", got)
	}
}

func TestIsUnread_NoSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if IsUnread("empty", "nope") {
		t.Error("IsUnread = true for nonexistent dir, want false")
	}
}
