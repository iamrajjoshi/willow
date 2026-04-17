package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/iamrajjoshi/willow/internal/claude"
)

// TestHookCmd_Structure checks that the subcommand is registered and hidden.
func TestHookCmd_Structure(t *testing.T) {
	cmd := hookCmd()
	if cmd.Name != "hook" {
		t.Errorf("Name = %q, want %q", cmd.Name, "hook")
	}
	if !cmd.Hidden {
		t.Error("hook subcommand should be hidden")
	}
}

// TestHookCmd_EndToEnd pipes a Stop event through the subcommand's Action
// and verifies the session status file is written.
func TestHookCmd_EndToEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repo, wt := "myrepo", "feat-x"
	wtPath := filepath.Join(home, ".willow", "worktrees", repo, wt)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(wtPath); err == nil {
		wtPath = resolved
	}
	prev, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(prev) })
	os.Chdir(wtPath)

	// Swap stdin with the hook event JSON.
	payload, _ := json.Marshal(claude.HookInput{
		SessionID:     "s1",
		HookEventName: "Stop",
	})
	r, w, _ := os.Pipe()
	w.Write(payload)
	w.Close()
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	// Silence stderr from fireNotifications.
	origErr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	t.Cleanup(func() { os.Stderr = origErr })

	cmd := hookCmd()
	if err := cmd.Action(context.Background(), cmd); err != nil {
		t.Fatalf("Action: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(claude.StatusDir(), repo, wt, "s1.json"))
	if err != nil {
		t.Fatalf("status file missing: %v", err)
	}
	if !bytes.Contains(data, []byte(`"status":"DONE"`)) {
		t.Errorf("status file does not mark DONE: %s", data)
	}
}
