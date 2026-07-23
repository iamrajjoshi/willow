package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/focus"
	"github.com/iamrajjoshi/willow/internal/notify"
)

func TestClickableNotificationDeadlineBudget(t *testing.T) {
	const cursorHookTimeout = 5 * time.Second
	if notifyKeyLockTimeout <= notify.ClickDispatchTimeout {
		t.Fatalf("key lock timeout %s must exceed in-lock dispatch timeout %s", notifyKeyLockTimeout, notify.ClickDispatchTimeout)
	}
	total := focusCaptureTimeout + notifyKeyLockTimeout + notify.ClickDispatchTimeout
	if total >= cursorHookTimeout {
		t.Fatalf("clickable notification budget %s must stay below Cursor hook timeout %s", total, cursorHookTimeout)
	}
}

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
				ts = detectNotificationTransitions("repo/wt", agg.Status, "claude/s1", NotifyStateFile())
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

func TestNotifyKeyLock_DifferentWorktreesDoNotBlock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	enteredFirst := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan struct{})
	go func() {
		_ = withNotifyKeyLock("repo/a", func() error {
			close(enteredFirst)
			<-releaseFirst
			return nil
		})
		close(firstDone)
	}()
	<-enteredFirst

	enteredSecond := make(chan struct{})
	go func() {
		_ = withNotifyKeyLock("repo/b", func() error {
			close(enteredSecond)
			return nil
		})
	}()
	select {
	case <-enteredSecond:
	case <-time.After(time.Second):
		close(releaseFirst)
		t.Fatal("a different worktree waited on the active dispatch lock")
	}
	close(releaseFirst)
	<-firstDone
}

func TestNotifyKeyLock_SerializesOneWorktree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	enteredFirst := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan struct{})
	go func() {
		_ = withNotifyKeyLock("repo/wt", func() error {
			close(enteredFirst)
			<-releaseFirst
			return nil
		})
		close(firstDone)
	}()
	<-enteredFirst

	enteredSecond := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		_ = withNotifyKeyLock("repo/wt", func() error {
			close(enteredSecond)
			return nil
		})
		close(secondDone)
	}()
	select {
	case <-enteredSecond:
		close(releaseFirst)
		t.Fatal("same-worktree dispatches entered concurrently")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)
	<-firstDone
	select {
	case <-enteredSecond:
	case <-time.After(time.Second):
		t.Fatal("second same-worktree dispatch did not run after release")
	}
	<-secondDone
}

func TestNotifyKeyLock_TimeoutDoesNotRunAction(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := notifyKeyLockFile("repo/wt")
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_ = withNotifyFileLock(path, func() error {
			close(entered)
			<-release
			return nil
		})
		close(done)
	}()
	<-entered

	ran := false
	err := withNotifyFileLockTimeout(path, 20*time.Millisecond, time.Millisecond, func() error {
		ran = true
		return nil
	})
	close(release)
	<-done
	if !errors.Is(err, errNotifyKeyLockTimeout) {
		t.Fatalf("lock error = %v, want timeout", err)
	}
	if ran {
		t.Fatal("timed-out notification lock ran its action")
	}
}

func TestFireNotifications_NewerTargetWaitsForGroupedDispatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	originalSupportsClick := supportsNotificationClick
	originalDispatch := dispatchNotificationTransitions
	t.Cleanup(func() {
		supportsNotificationClick = originalSupportsClick
		dispatchNotificationTransitions = originalDispatch
	})
	supportsNotificationClick = func() bool { return true }

	var mu sync.Mutex
	var targets []string
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	dispatchNotificationTransitions = func(_ *config.Config, _ string, _ []Transition, target focus.Target, _ bool) {
		mu.Lock()
		targets = append(targets, target.TermBundle)
		first := len(targets) == 1
		mu.Unlock()
		if first {
			close(firstStarted)
			<-releaseFirst
		}
	}

	const repo, wt, harnessID, sessionID = "repo", "wt", "claude", "session"
	path := SessionPath(repo, wt, harnessID, sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	writeTarget := func(bundle string) {
		t.Helper()
		target := &focus.Target{Session: repo + "/" + wt, TermBundle: bundle}
		if err := writeSession(path, SessionStatus{
			Harness:     harnessID,
			SessionID:   sessionID,
			Status:      StatusWait,
			Timestamp:   time.Now().UTC(),
			Worktree:    wt,
			FocusTarget: target,
		}); err != nil {
			t.Fatalf("write session: %v", err)
		}
	}

	detectNotificationTransitions(repo+"/"+wt, StatusBusy, "", NotifyStateFile())
	writeTarget("com.apple.Terminal")
	firstDone := make(chan struct{})
	go func() {
		fireNotifications(repo, wt)
		close(firstDone)
	}()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first grouped dispatch did not start")
	}

	writeTarget("com.googlecode.iterm2")
	secondDone := make(chan struct{})
	go func() {
		fireNotifications(repo, wt)
		close(secondDone)
	}()
	time.Sleep(800 * time.Millisecond)
	close(releaseFirst)

	for name, done := range map[string]<-chan struct{}{"first": firstDone, "second": secondDone} {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s notification dispatch did not finish", name)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if got := strings.Join(targets, ","); got != "com.apple.Terminal,com.googlecode.iterm2" {
		t.Fatalf("grouped dispatch targets = %q, want old then latest", got)
	}
}

func TestDetectNotificationTransitions_ReplacesWhenTargetChanges(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "notify-state.json")

	// Start from the legacy string-only state format to cover upgrades.
	DetectTransitions(map[string]Status{"repo/wt": StatusBusy}, stateFile)
	ts := detectNotificationTransitions("repo/wt", StatusWait, "claude/session-a", stateFile)
	if len(ts) != 1 || ts[0].FromStatus != StatusBusy || ts[0].ToStatus != StatusWait || ts[0].Refresh {
		t.Fatalf("BUSY→WAIT transition = %#v, want one notification", ts)
	}

	if ts := detectNotificationTransitions("repo/wt", StatusWait, "claude/session-a", stateFile); len(ts) != 0 {
		t.Fatalf("unchanged WAIT target produced transitions: %#v", ts)
	}

	ts = detectNotificationTransitions("repo/wt", StatusWait, "claude/session-b", stateFile)
	if len(ts) != 1 || ts[0].FromStatus != StatusWait || ts[0].ToStatus != StatusWait || !ts[0].Refresh {
		t.Fatalf("WAIT target replacement = %#v, want one replacement notification", ts)
	}
}
