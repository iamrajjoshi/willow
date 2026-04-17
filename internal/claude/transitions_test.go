package claude

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestDetectTransitions_PreservesSiblingKeys ensures that when a single-hook
// invocation updates one worktree's state, it does not erase the recorded
// state for other worktrees. The per-hook dispatch path depends on this so
// concurrent worktrees don't lose track of each other's BUSY status.
func TestDetectTransitions_PreservesSiblingKeys(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")

	// Worktree A goes BUSY.
	DetectTransitions(map[string]Status{"repo/A": StatusBusy}, stateFile)
	// Worktree B goes BUSY (separate hook, separate call).
	DetectTransitions(map[string]Status{"repo/B": StatusBusy}, stateFile)

	// Worktree A transitions to DONE. B's BUSY must still be tracked so its
	// eventual transition to DONE gets detected.
	ts := DetectTransitions(map[string]Status{"repo/A": StatusDone}, stateFile)
	if len(ts) != 1 || ts[0].Key != "repo/A" || ts[0].ToStatus != StatusDone {
		t.Fatalf("A transition = %v, want A→DONE", ts)
	}

	// Now B transitions to DONE. This must fire — regression guard.
	ts = DetectTransitions(map[string]Status{"repo/B": StatusDone}, stateFile)
	if len(ts) != 1 || ts[0].Key != "repo/B" || ts[0].ToStatus != StatusDone {
		t.Fatalf("B transition = %v, want B→DONE (sibling state was clobbered)", ts)
	}
}

// TestFireNotifications_SingleTransitionUnderLock spawns concurrent goroutines
// calling fireNotifications for the same worktree. The underlying
// DetectTransitions should report exactly one BUSY→DONE transition across all
// invocations, not N copies — the flock guarantees at-most-once semantics for
// the shared state file.
func TestFireNotifications_SingleTransitionUnderLock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Seed a session file first (this creates the worktree status dir tree).
	sessDir := filepath.Join(StatusDir(), "repo", "wt")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Seed state: one worktree was BUSY. The state file lives under
	// ~/.willow/, which StatusDir() has now implicitly ensured exists.
	DetectTransitions(map[string]Status{"repo/wt": StatusBusy}, NotifyStateFile())
	if err := writeSession(filepath.Join(sessDir, "s1.json"), SessionStatus{
		Status:    StatusDone,
		SessionID: "s1",
		Worktree:  "wt",
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	// Race N goroutines through the locked section.
	var count int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var ts []Transition
			_ = withNotifyLock(func() error {
				sessions := ReadAllSessions("repo", "wt")
				agg := AggregateStatus(sessions)
				ts = DetectTransitions(
					map[string]Status{"repo/wt": agg.Status},
					NotifyStateFile(),
				)
				return nil
			})
			mu.Lock()
			count += len(ts)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if count != 1 {
		t.Errorf("observed %d transitions across racing goroutines, want exactly 1", count)
	}
}

