package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadStatus_MissingFile(t *testing.T) {
	ws := ReadStatus("nonexistent-repo", "nonexistent-wt")
	if ws.Status != StatusOffline {
		t.Errorf("Status = %q, want %q", ws.Status, StatusOffline)
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

func TestReadAllSessions_PreservesOldFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "old-sessions"
	sessDir := filepath.Join(home, ".willow", "status", repoName, wtName)
	os.MkdirAll(sessDir, 0o755)

	// Write a session file with a timestamp older than 30 minutes
	old := SessionStatus{Status: StatusDone, SessionID: "old-sess", Timestamp: time.Now().UTC().Add(-2 * time.Hour)}
	data, _ := json.Marshal(old)
	os.WriteFile(filepath.Join(sessDir, old.SessionID+".json"), data, 0o644)

	got := ReadAllSessions(repoName, wtName)
	if len(got) != 1 {
		t.Fatalf("ReadAllSessions returned %d sessions, want 1 (should preserve old files)", len(got))
	}

	// Verify file still exists on disk
	if _, err := os.Stat(filepath.Join(sessDir, "old-sess.json")); os.IsNotExist(err) {
		t.Error("ReadAllSessions deleted old session file, but should only read")
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

func TestStatusOrder_Ordering(t *testing.T) {
	if StatusOrder(StatusBusy) >= StatusOrder(StatusWait) {
		t.Error("BUSY should have higher priority than WAIT")
	}
	if StatusOrder(StatusWait) >= StatusOrder(StatusDone) {
		t.Error("WAIT should have higher priority than DONE")
	}
	if StatusOrder(StatusDone) >= StatusOrder(StatusIdle) {
		t.Error("DONE should have higher priority than IDLE")
	}
	if StatusOrder(StatusIdle) >= StatusOrder(StatusOffline) {
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

func TestHookScript_ContainsEnrichmentFields(t *testing.T) {
	script := HookScript()
	for _, field := range []string{"tool_count", "start_time", ".files"} {
		if !strings.Contains(script, field) {
			t.Errorf("HookScript() missing enrichment field %q", field)
		}
	}
}

func TestSessionStatus_EnrichedFields(t *testing.T) {
	now := time.Now().UTC()
	ss := SessionStatus{
		Status:    StatusBusy,
		SessionID: "s1",
		Timestamp: now,
		Worktree:  "wt",
		ToolCount: 42,
		StartTime: now.Add(-10 * time.Minute),
	}
	data, err := json.Marshal(ss)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got SessionStatus
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ToolCount != 42 {
		t.Errorf("ToolCount = %d, want 42", got.ToolCount)
	}
	if got.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}
}

func TestReadFilesTouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtName := "wt1"
	sessionID := "sess-1"
	dir := filepath.Join(StatusDir(), repoName, wtName)
	os.MkdirAll(dir, 0o755)

	// Write a .files sidecar
	content := "/path/to/file1.go\n/path/to/file2.go\n/path/to/file1.go\n"
	os.WriteFile(filepath.Join(dir, sessionID+".files"), []byte(content), 0o644)

	got := ReadFilesTouched(repoName, wtName, sessionID)
	if len(got) != 2 {
		t.Fatalf("ReadFilesTouched returned %d files, want 2 (deduplicated)", len(got))
	}
	if got[0] != "/path/to/file1.go" || got[1] != "/path/to/file2.go" {
		t.Errorf("unexpected files: %v", got)
	}
}

func TestReadFilesTouched_Missing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := ReadFilesTouched("nope", "nope", "nope")
	if got != nil {
		t.Errorf("expected nil for missing file, got %v", got)
	}
}
