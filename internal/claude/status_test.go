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

func TestReadStatus_ValidFile(t *testing.T) {
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

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusBusy, "\U0001F916"},
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
