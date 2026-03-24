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

func TestReadStatus_StaleDoneStaysDone(t *testing.T) {
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
	if got.Status != StatusDone {
		t.Errorf("Status = %q, want %q (stale DONE should stay DONE)", got.Status, StatusDone)
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

	if got := TimeSince(time.Now().Add(-3 * time.Hour)); got != "3h ago" {
		t.Errorf("TimeSince(3h) = %q, want '3h ago'", got)
	}

	if got := TimeSince(time.Now().Add(-48 * time.Hour)); got != "2d ago" {
		t.Errorf("TimeSince(48h) = %q, want '2d ago'", got)
	}
}

func TestEffectiveStatus_Fresh(t *testing.T) {
	now := time.Now()
	if got := EffectiveStatus(StatusBusy, now); got != StatusBusy {
		t.Errorf("fresh BUSY = %q, want BUSY", got)
	}
	if got := EffectiveStatus(StatusDone, now); got != StatusDone {
		t.Errorf("fresh DONE = %q, want DONE", got)
	}
	if got := EffectiveStatus(StatusWait, now); got != StatusWait {
		t.Errorf("fresh WAIT = %q, want WAIT", got)
	}
	if got := EffectiveStatus(StatusIdle, now); got != StatusIdle {
		t.Errorf("fresh IDLE = %q, want IDLE", got)
	}
}

func TestEffectiveStatus_Stale(t *testing.T) {
	stale := time.Now().Add(-5 * time.Minute)
	if got := EffectiveStatus(StatusBusy, stale); got != StatusIdle {
		t.Errorf("stale BUSY = %q, want IDLE", got)
	}
	if got := EffectiveStatus(StatusDone, stale); got != StatusDone {
		t.Errorf("stale DONE = %q, want DONE", got)
	}
	if got := EffectiveStatus(StatusWait, stale); got != StatusIdle {
		t.Errorf("stale WAIT = %q, want IDLE", got)
	}
	// IDLE stays IDLE regardless of staleness
	if got := EffectiveStatus(StatusIdle, stale); got != StatusIdle {
		t.Errorf("stale IDLE = %q, want IDLE", got)
	}
}

func TestStatusPriority_Ordering(t *testing.T) {
	if statusPriority(StatusBusy) >= statusPriority(StatusWait) {
		t.Error("BUSY should have higher priority than WAIT")
	}
	if statusPriority(StatusWait) >= statusPriority(StatusDone) {
		t.Error("WAIT should have higher priority than DONE")
	}
	if statusPriority(StatusDone) >= statusPriority(StatusIdle) {
		t.Error("DONE should have higher priority than IDLE")
	}
	if statusPriority(StatusIdle) >= statusPriority(StatusOffline) {
		t.Error("IDLE should have higher priority than OFFLINE")
	}
}

func TestReadStatus_WaitAggregation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "wait-test"
	sessDir := filepath.Join(home, ".willow", "status", repoName, wtName)
	os.MkdirAll(sessDir, 0o755)

	now := time.Now().UTC()
	for _, ss := range []SessionStatus{
		{Status: StatusWait, SessionID: "s1", Timestamp: now},
		{Status: StatusDone, SessionID: "s2", Timestamp: now},
	} {
		data, _ := json.Marshal(ss)
		os.WriteFile(filepath.Join(sessDir, ss.SessionID+".json"), data, 0o644)
	}

	got := ReadStatus(repoName, wtName)
	if got.Status != StatusWait {
		t.Errorf("aggregate Status = %q, want %q (WAIT should win over DONE)", got.Status, StatusWait)
	}
}

func TestHookScript_NotEmpty(t *testing.T) {
	script := HookScript()
	if len(script) == 0 {
		t.Error("HookScript() returned empty string")
	}
}
