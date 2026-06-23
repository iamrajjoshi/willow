package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iamrajjoshi/willow/internal/agent"
)

func TestRefreshStatusDryRunAndRemoval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeActiveSessionFile(t, "repo", "feature", "s1", agent.StatusBusy)
	sessionPath := filepath.Join(agent.StatusDir(), "repo", "feature", "s1.json")

	dryRunOut, err := captureStdout(t, func() error {
		return runApp("refresh-status", "--dry-run")
	})
	if err != nil {
		t.Fatalf("refresh-status --dry-run failed: %v", err)
	}
	if !strings.Contains(dryRunOut, "Would remove repo/feature session s1") {
		t.Fatalf("dry-run output missing orphaned session:\n%s", dryRunOut)
	}
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("dry-run should leave session file in place: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runApp("refresh-status")
	})
	if err != nil {
		t.Fatalf("refresh-status failed: %v", err)
	}
	if !strings.Contains(out, "Removed 1 orphaned session") {
		t.Fatalf("remove output missing summary:\n%s", out)
	}
	if _, err := os.Stat(sessionPath); !os.IsNotExist(err) {
		t.Fatalf("session file should be removed, stat err = %v", err)
	}
}

func TestRefreshStatusRemovesOnlyMatchingHarnessSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "repo"
	wtName := "feature"
	sessionID := "shared-session"

	writeHarnessSessionFile(t, repoName, wtName, "claude", agent.SessionStatus{
		Harness:   "claude",
		SessionID: sessionID,
		Status:    agent.StatusDone,
		Timestamp: time.Now().UTC(),
	})
	writeHarnessSessionFile(t, repoName, wtName, "codex", agent.SessionStatus{
		Harness:   "codex",
		SessionID: sessionID,
		Status:    agent.StatusBusy,
		Timestamp: time.Now().UTC(),
	})

	out, err := captureStdout(t, func() error {
		return runApp("refresh-status")
	})
	if err != nil {
		t.Fatalf("refresh-status failed: %v", err)
	}
	if !strings.Contains(out, "Removed 1 orphaned session") {
		t.Fatalf("remove output missing summary:\n%s", out)
	}

	if _, err := os.Stat(agent.SessionPath(repoName, wtName, "claude", sessionID)); err != nil {
		t.Fatalf("claude session should remain: %v", err)
	}
	if _, err := os.Stat(agent.SessionPath(repoName, wtName, "codex", sessionID)); !os.IsNotExist(err) {
		t.Fatalf("codex session should be removed, stat err = %v", err)
	}
}

func writeHarnessSessionFile(t *testing.T, repoName, wtName, harnessID string, session agent.SessionStatus) {
	t.Helper()
	path := agent.SessionPath(repoName, wtName, harnessID, session.SessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write session file: %v", err)
	}
}
