package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadStatus_MissingFile(t *testing.T) {
	ws := ReadStatus("nonexistent-repo", "nonexistent-wt")
	if ws.Status != StatusOffline {
		t.Errorf("Status = %q, want %q", ws.Status, StatusOffline)
	}
}

func TestReadStatus_LegacyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "feature-auth"
	statusDir := filepath.Join(home, ".willow", "status", repoName)
	os.MkdirAll(statusDir, 0o755)

	ws := WorktreeStatus{
		Status:    StatusBusy,
		Timestamp: time.Now().UTC(),
		Worktree:  wtName,
	}
	data, _ := json.Marshal(ws)
	os.WriteFile(filepath.Join(statusDir, wtName+".json"), data, 0o644)

	got := ReadStatus(repoName, wtName)
	if got.Status != StatusBusy {
		t.Errorf("Status = %q, want %q", got.Status, StatusBusy)
	}
}

func TestReadStatus_StaleBusyBecomesIdle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "stale-wt"
	statusDir := filepath.Join(home, ".willow", "status", repoName)
	os.MkdirAll(statusDir, 0o755)

	ws := WorktreeStatus{
		Status:    StatusBusy,
		Timestamp: time.Now().UTC().Add(-10 * time.Minute),
		Worktree:  wtName,
	}
	data, _ := json.Marshal(ws)
	os.WriteFile(filepath.Join(statusDir, wtName+".json"), data, 0o644)

	got := ReadStatus(repoName, wtName)
	if got.Status != StatusIdle {
		t.Errorf("Status = %q, want %q (stale BUSY should become IDLE)", got.Status, StatusIdle)
	}
}

func TestReadStatus_StaleDoneBecomesIdle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "stale-done-wt"
	statusDir := filepath.Join(home, ".willow", "status", repoName)
	os.MkdirAll(statusDir, 0o755)

	ws := WorktreeStatus{
		Status:    StatusDone,
		Timestamp: time.Now().UTC().Add(-10 * time.Minute),
		Worktree:  wtName,
	}
	data, _ := json.Marshal(ws)
	os.WriteFile(filepath.Join(statusDir, wtName+".json"), data, 0o644)

	got := ReadStatus(repoName, wtName)
	if got.Status != StatusIdle {
		t.Errorf("Status = %q, want %q (stale DONE should become IDLE)", got.Status, StatusIdle)
	}
}

func TestReadStatus_InvalidJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "bad-json"
	statusDir := filepath.Join(home, ".willow", "status", repoName)
	os.MkdirAll(statusDir, 0o755)
	os.WriteFile(filepath.Join(statusDir, wtName+".json"), []byte("{invalid"), 0o644)

	got := ReadStatus(repoName, wtName)
	if got.Status != StatusOffline {
		t.Errorf("Status = %q, want %q", got.Status, StatusOffline)
	}
}

func TestReadAllSessions_MultipleSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "feature-auth"
	sessDir := filepath.Join(home, ".willow", "status", repoName, wtName)
	os.MkdirAll(sessDir, 0o755)

	now := time.Now().UTC()
	sessions := []SessionStatus{
		{Status: StatusBusy, SessionID: "sess-1", Timestamp: now, Worktree: wtName},
		{Status: StatusDone, SessionID: "sess-2", Timestamp: now.Add(-1 * time.Minute), Worktree: wtName},
	}
	for _, ss := range sessions {
		data, _ := json.Marshal(ss)
		os.WriteFile(filepath.Join(sessDir, ss.SessionID+".json"), data, 0o644)
	}

	got := ReadAllSessions(repoName, wtName)
	if len(got) != 2 {
		t.Fatalf("ReadAllSessions returned %d sessions, want 2", len(got))
	}
}

func TestReadAllSessions_EmptyDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sessDir := filepath.Join(home, ".willow", "status", "myrepo", "empty-wt")
	os.MkdirAll(sessDir, 0o755)

	got := ReadAllSessions("myrepo", "empty-wt")
	if len(got) != 0 {
		t.Errorf("ReadAllSessions returned %d sessions, want 0", len(got))
	}
}

func TestReadAllSessions_NoDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := ReadAllSessions("nonexistent", "nope")
	if got != nil {
		t.Errorf("ReadAllSessions returned %v, want nil", got)
	}
}

func TestReadStatus_AggregatesBusyOverDone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "multi"
	sessDir := filepath.Join(home, ".willow", "status", repoName, wtName)
	os.MkdirAll(sessDir, 0o755)

	now := time.Now().UTC()
	for _, ss := range []SessionStatus{
		{Status: StatusDone, SessionID: "s1", Timestamp: now},
		{Status: StatusBusy, SessionID: "s2", Timestamp: now},
	} {
		data, _ := json.Marshal(ss)
		os.WriteFile(filepath.Join(sessDir, ss.SessionID+".json"), data, 0o644)
	}

	got := ReadStatus(repoName, wtName)
	if got.Status != StatusBusy {
		t.Errorf("aggregate Status = %q, want %q (BUSY should win)", got.Status, StatusBusy)
	}
}

func TestCleanStaleSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "cleanup"
	sessDir := filepath.Join(home, ".willow", "status", repoName, wtName)
	os.MkdirAll(sessDir, 0o755)

	stale := SessionStatus{Status: StatusDone, SessionID: "old", Timestamp: time.Now().UTC().Add(-1 * time.Hour)}
	fresh := SessionStatus{Status: StatusBusy, SessionID: "new", Timestamp: time.Now().UTC()}

	for _, ss := range []SessionStatus{stale, fresh} {
		data, _ := json.Marshal(ss)
		os.WriteFile(filepath.Join(sessDir, ss.SessionID+".json"), data, 0o644)
	}

	CleanStaleSessions(repoName, wtName)

	entries, _ := os.ReadDir(sessDir)
	jsonFiles := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			jsonFiles++
		}
	}
	if jsonFiles != 1 {
		t.Errorf("after cleanup: %d json files, want 1", jsonFiles)
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusBusy, "\U0001F916"},
		{StatusDone, "\u2705"},
		{StatusWait, "\u23F3"},
		{StatusIdle, "\U0001F7E1"},
		{StatusOffline, "  "},
	}
	for _, tt := range tests {
		got := StatusIcon(tt.status)
		if got != tt.want {
			t.Errorf("StatusIcon(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestTimeSince(t *testing.T) {
	if got := TimeSince(time.Time{}); got != "" {
		t.Errorf("TimeSince(zero) = %q, want empty", got)
	}

	if got := TimeSince(time.Now().Add(-30 * time.Second)); got != "just now" {
		t.Errorf("TimeSince(30s) = %q, want 'just now'", got)
	}

	if got := TimeSince(time.Now().Add(-5 * time.Minute)); got != "5m ago" {
		t.Errorf("TimeSince(5m) = %q, want '5m ago'", got)
	}
}

func TestHookScript_NotEmpty(t *testing.T) {
	script := HookScript()
	if len(script) == 0 {
		t.Error("HookScript() returned empty string")
	}
}
