package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/claude"
)

func TestRefreshStatusDryRunAndRemoval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeActiveSessionFile(t, "repo", "feature", "s1", claude.StatusBusy)
	sessionPath := filepath.Join(claude.StatusDir(), "repo", "feature", "s1.json")

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
