package focus

import (
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeTerminalDriver struct {
	id                 string
	focusClientFunc    func(terminalClientTarget) error
	focusTitleFunc     func(string) error
	openAttachmentFunc func(string) error
}

func (d fakeTerminalDriver) bundleID() string {
	return d.id
}

func (d fakeTerminalDriver) focusClientExact(target terminalClientTarget) error {
	if d.focusClientFunc == nil {
		return errTerminalNotFound
	}
	return d.focusClientFunc(target)
}

func (d fakeTerminalDriver) focusTitle(title string) error {
	if d.focusTitleFunc == nil {
		return errTerminalNotFound
	}
	return d.focusTitleFunc(title)
}

func (d fakeTerminalDriver) openAttachment(command string) error {
	if d.openAttachmentFunc == nil {
		return errTerminalNotFound
	}
	return d.openAttachmentFunc(command)
}

func withTerminalDriverRegistry(t *testing.T, drivers []terminalDriver) {
	t.Helper()
	original := terminalDriverRegistry
	terminalDriverRegistry = drivers
	t.Cleanup(func() {
		terminalDriverRegistry = original
	})
}

func driverBundleIDs(drivers []terminalClientProbe) []string {
	ids := make([]string, 0, len(drivers))
	for _, driver := range drivers {
		ids = append(ids, driver.bundleID())
	}
	return ids
}

func TestTerminalDriversForClient_PrefersSupportedBundleAndDeduplicates(t *testing.T) {
	withTerminalDriverRegistry(t, []terminalDriver{
		fakeTerminalDriver{id: "terminal-a"},
		fakeTerminalDriver{id: "terminal-b"},
		fakeTerminalDriver{id: "terminal-b"},
		fakeTerminalDriver{id: "terminal-c"},
	})

	tests := []struct {
		name      string
		preferred string
		want      string
	}{
		{name: "registered preference", preferred: "terminal-b", want: "terminal-b,terminal-a,terminal-c"},
		{name: "first driver preferred", preferred: "terminal-a", want: "terminal-a,terminal-b,terminal-c"},
		{name: "unknown preference", preferred: "stale-terminal", want: "terminal-a,terminal-b,terminal-c"},
		{name: "empty preference", want: "terminal-a,terminal-b,terminal-c"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := strings.Join(driverBundleIDs(terminalDriversForClient(test.preferred)), ",")
			if got != test.want {
				t.Fatalf("terminalDriversForClient(%q) = %q, want %q", test.preferred, got, test.want)
			}
		})
	}
}

func TestSelectTerminal_ProbesInOrderAndStopsAfterSuccess(t *testing.T) {
	var calls []string
	var gotTarget terminalClientTarget
	driver := func(id string, result error) terminalDriver {
		return fakeTerminalDriver{
			id: id,
			focusClientFunc: func(target terminalClientTarget) error {
				calls = append(calls, id)
				gotTarget = target
				return result
			},
		}
	}
	withTerminalDriverRegistry(t, []terminalDriver{
		driver("terminal-a", nil),
		driver("terminal-b", errTerminalNotFound),
		driver("terminal-c", nil),
	})

	err := selectTerminal("/dev/ttys007", "repo/wt", "terminal-b", "xterm-example", "/worktrees/repo/wt")
	if err != nil {
		t.Fatalf("selectTerminal returned error: %v", err)
	}
	if got := strings.Join(calls, ","); got != "terminal-b,terminal-a" {
		t.Fatalf("focusClient calls = %q, want preferred driver then first successful fallback", got)
	}
	wantTarget := terminalClientTarget{
		tty:            "/dev/ttys007",
		title:          "repo/wt",
		termName:       "xterm-example",
		currentPath:    "/worktrees/repo/wt",
		capturedBundle: "terminal-b",
	}
	if gotTarget != wantTarget {
		t.Fatalf("focusClient target = %#v, want %#v", gotTarget, wantTarget)
	}
}

func TestSelectTerminal_UnknownCapturedBundleUsesRegistryOrder(t *testing.T) {
	var calls []string
	driver := func(id string, result error) terminalDriver {
		return fakeTerminalDriver{
			id: id,
			focusClientFunc: func(terminalClientTarget) error {
				calls = append(calls, id)
				return result
			},
		}
	}
	withTerminalDriverRegistry(t, []terminalDriver{
		driver("terminal-a", errTerminalNotFound),
		driver("terminal-b", nil),
		driver("terminal-c", nil),
	})

	if err := selectTerminal("/dev/ttys007", "repo/wt", "stale-terminal", "xterm-example", "/worktrees/repo/wt"); err != nil {
		t.Fatalf("selectTerminal returned error: %v", err)
	}
	if got := strings.Join(calls, ","); got != "terminal-a,terminal-b" {
		t.Fatalf("focusClient calls = %q, want registry order through the first successful driver", got)
	}
}

type fakeCompatibilityTerminalDriver struct {
	fakeTerminalDriver
	focusClientCompatibilityFunc func(terminalClientTarget) error
}

func (d fakeCompatibilityTerminalDriver) focusClientCompatibility(target terminalClientTarget) error {
	if d.focusClientCompatibilityFunc == nil {
		return errTerminalNotFound
	}
	return d.focusClientCompatibilityFunc(target)
}

func TestSelectTerminal_ProbesEveryExactTTYBeforeCompatibilityFallback(t *testing.T) {
	var calls []string
	withTerminalDriverRegistry(t, []terminalDriver{
		fakeTerminalDriver{
			id: "exact-terminal",
			focusClientFunc: func(terminalClientTarget) error {
				calls = append(calls, "exact-terminal:exact")
				return nil
			},
		},
		fakeCompatibilityTerminalDriver{
			fakeTerminalDriver: fakeTerminalDriver{
				id: "compatibility-terminal",
				focusClientFunc: func(terminalClientTarget) error {
					calls = append(calls, "compatibility-terminal:exact")
					return errTerminalNotFound
				},
			},
			focusClientCompatibilityFunc: func(terminalClientTarget) error {
				calls = append(calls, "compatibility-terminal:fallback")
				return nil
			},
		},
	})

	if err := selectTerminal("/dev/ttys007", "repo/wt", "compatibility-terminal", "xterm-256color", "/worktrees/repo/wt"); err != nil {
		t.Fatalf("selectTerminal returned error: %v", err)
	}
	if got := strings.Join(calls, ","); got != "compatibility-terminal:exact,exact-terminal:exact" {
		t.Fatalf("focusClient calls = %q, want all exact probes to precede and suppress compatibility fallback", got)
	}
}

func TestSelectTerminal_UsesCompatibilityFallbackAfterExactTTYProbes(t *testing.T) {
	var calls []string
	compatibilityDriver := func(id string) terminalDriver {
		return fakeCompatibilityTerminalDriver{
			fakeTerminalDriver: fakeTerminalDriver{
				id: id,
				focusClientFunc: func(terminalClientTarget) error {
					calls = append(calls, id+":exact")
					return errTerminalNotFound
				},
			},
			focusClientCompatibilityFunc: func(terminalClientTarget) error {
				calls = append(calls, id+":fallback")
				if id == "preferred-terminal" {
					return nil
				}
				return errTerminalNotFound
			},
		}
	}
	withTerminalDriverRegistry(t, []terminalDriver{
		compatibilityDriver("other-terminal"),
		compatibilityDriver("preferred-terminal"),
	})

	if err := selectTerminal("/dev/ttys007", "repo/wt", "preferred-terminal", "xterm-256color", "/worktrees/repo/wt"); err != nil {
		t.Fatalf("selectTerminal returned error: %v", err)
	}
	if got := strings.Join(calls, ","); got != "preferred-terminal:exact,other-terminal:exact,preferred-terminal:fallback" {
		t.Fatalf("focusClient calls = %q, want preferred exact, remaining exact, then preferred fallback", got)
	}
}

func TestGhosttyCompatibility_AcceptsGenericTermForCapturedGhostty(t *testing.T) {
	withTerminalDriverRegistry(t, []terminalDriver{ghosttyTerminalDriver{}})
	originalOutput := runCmdOutput
	t.Cleanup(func() {
		runCmdOutput = originalOutput
	})
	var scripts []string
	runCmdOutput = func(name string, args ...string) ([]byte, error) {
		if name != osascriptPath {
			return nil, errors.New("unexpected command")
		}
		script := strings.Join(args, " ")
		scripts = append(scripts, script)
		if strings.Contains(script, "name of term") {
			return []byte(selectedMarker + "\n"), nil
		}
		return []byte("not found\n"), nil
	}

	if err := selectTerminal("/dev/ttys007", "repo/wt", bundleGhostty, "xterm-256color", ""); err != nil {
		t.Fatalf("selectTerminal returned error for captured Ghostty with generic TERM: %v", err)
	}
	if len(scripts) != 2 || !strings.Contains(scripts[0], "tty of term") || !strings.Contains(scripts[1], "name of term") {
		t.Fatalf("scripts = %#v, want exact Ghostty TTY probe followed by title compatibility fallback", scripts)
	}
}

func TestGhosttyCompatibility_UsesOSC7ForCapturedGhosttyWithGenericTerm(t *testing.T) {
	withTerminalDriverRegistry(t, []terminalDriver{ghosttyTerminalDriver{}})
	originalOutput, originalWrite := runCmdOutput, writeTerminalSequence
	t.Cleanup(func() {
		runCmdOutput, writeTerminalSequence = originalOutput, originalWrite
	})
	var scripts []string
	runCmdOutput = func(name string, args ...string) ([]byte, error) {
		if name != osascriptPath {
			return nil, errors.New("unexpected command")
		}
		script := strings.Join(args, " ")
		scripts = append(scripts, script)
		switch {
		case strings.Contains(script, "NSJSONSerialization"):
			return []byte("{}"), nil
		case strings.Contains(script, "working directory of term"):
			return []byte("ghostty-id\n"), nil
		default:
			return []byte("not found\n"), nil
		}
	}
	var sequences []string
	writeTerminalSequence = func(tty string, sequence []byte) error {
		if tty != "/dev/ttys007" {
			t.Fatalf("writeTerminalSequence tty = %q, want /dev/ttys007", tty)
		}
		sequences = append(sequences, string(sequence))
		return nil
	}

	if err := selectTerminal("/dev/ttys007", "repo/wt", bundleGhostty, "xterm-256color", "/worktrees/repo/wt"); err != nil {
		t.Fatalf("selectTerminal returned error for captured Ghostty with generic TERM: %v", err)
	}
	if len(sequences) != 2 || !strings.Contains(sequences[0], "/tmp/willow-focus-ttys007") || !strings.Contains(sequences[1], "/worktrees/repo/wt") {
		t.Fatalf("terminal sequences = %#v, want OSC 7 marker and working-directory restore", sequences)
	}
	for _, script := range scripts {
		if strings.Contains(script, "name of term") {
			t.Fatalf("title fallback ran after OSC 7 selected the terminal: %q", script)
		}
	}
}

func TestGhosttyCompatibility_DoesNotPrecedeAnotherDriversExactTTYMatch(t *testing.T) {
	var calls []string
	withTerminalDriverRegistry(t, []terminalDriver{
		fakeTerminalDriver{
			id: bundleITerm2,
			focusClientFunc: func(terminalClientTarget) error {
				calls = append(calls, "iterm-exact")
				return nil
			},
		},
		ghosttyTerminalDriver{},
	})
	originalOutput := runCmdOutput
	t.Cleanup(func() {
		runCmdOutput = originalOutput
	})
	runCmdOutput = func(name string, args ...string) ([]byte, error) {
		if name != osascriptPath {
			return nil, errors.New("unexpected command")
		}
		script := strings.Join(args, " ")
		if strings.Contains(script, "tty of term") {
			calls = append(calls, "ghostty-exact")
		}
		if strings.Contains(script, "name of term") {
			calls = append(calls, "ghostty-fallback")
		}
		return []byte("not found\n"), nil
	}

	if err := selectTerminal("/dev/ttys007", "repo/wt", bundleGhostty, "xterm-256color", ""); err != nil {
		t.Fatalf("selectTerminal returned error: %v", err)
	}
	if got := strings.Join(calls, ","); got != "ghostty-exact,iterm-exact" {
		t.Fatalf("focusClient calls = %q, want another driver's exact TTY match before Ghostty fallback", got)
	}
}

func TestSelectTab_RoutesOnlyToCapturedDriver(t *testing.T) {
	var calls []string
	withTerminalDriverRegistry(t, []terminalDriver{
		fakeTerminalDriver{
			id: "terminal-a",
			focusTitleFunc: func(string) error {
				calls = append(calls, "terminal-a")
				return nil
			},
		},
		fakeTerminalDriver{
			id: "terminal-b",
			focusTitleFunc: func(title string) error {
				calls = append(calls, "terminal-b:"+title)
				return nil
			},
		},
	})

	if err := selectTab(Target{Session: "repo/wt", TermBundle: "terminal-b"}); err != nil {
		t.Fatalf("selectTab returned error: %v", err)
	}
	if got := strings.Join(calls, ","); got != "terminal-b:repo/wt" {
		t.Fatalf("focusTitle calls = %q, want only the captured terminal", got)
	}
}

func TestOpenTmuxSession_RoutesExactCommandOnlyToCapturedDriver(t *testing.T) {
	var calls []string
	withTerminalDriverRegistry(t, []terminalDriver{
		fakeTerminalDriver{
			id: "terminal-a",
			openAttachmentFunc: func(command string) error {
				calls = append(calls, "terminal-a:"+command)
				return nil
			},
		},
		fakeTerminalDriver{
			id: "terminal-b",
			openAttachmentFunc: func(command string) error {
				calls = append(calls, "terminal-b:"+command)
				return nil
			},
		},
	})
	target := Target{
		Session:    "repo/wt",
		TmuxPath:   "/opt/homebrew/bin/tmux",
		TmuxSocket: "/tmp/socket with spaces",
		TermBundle: "terminal-b",
	}

	if err := openTmuxSession(target); err != nil {
		t.Fatalf("openTmuxSession returned error: %v", err)
	}
	want := "terminal-b:" + tmuxAttachCommand(target)
	if got := strings.Join(calls, ","); got != want {
		t.Fatalf("openAttachment calls = %q, want %q", got, want)
	}
}

func TestTmuxLeasedAttachCommand_DelayedOldLeaseCannotAttach(t *testing.T) {
	target := Target{
		Session:    "repo/wt",
		TmuxPath:   "/opt/homebrew/bin/tmux",
		TmuxSocket: "/tmp/socket with spaces",
	}
	oldLease := tmuxAttachmentLease{value: "1000:old-owner"}
	replacement := tmuxAttachmentLease{value: "2000:new-owner"}
	oldCommand := tmuxLeasedAttachCommand(target, oldLease)
	replacementCommand := tmuxLeasedAttachCommand(target, replacement)

	authorizedBy := func(command string, lease tmuxAttachmentLease) bool {
		return strings.Contains(command, shellQuote(tmuxAttachmentLeaseCondition(lease)))
	}
	if authorizedBy(oldCommand, replacement) {
		t.Fatalf("delayed old command is authorized by replacement lease: %q", oldCommand)
	}
	if !authorizedBy(replacementCommand, replacement) {
		t.Fatalf("replacement command is missing its lease guard: %q", replacementCommand)
	}
	for _, part := range []string{"if-shell", "-F", "attach-session"} {
		if !strings.Contains(replacementCommand, part) {
			t.Fatalf("replacement command %q is missing %q", replacementCommand, part)
		}
	}
	if strings.Contains(replacementCommand, "'=repo/wt'") || strings.Contains(replacementCommand, "attach-session -t") {
		t.Fatalf("leased opener should not depend on the original session: %q", replacementCommand)
	}
	if strings.Contains(replacementCommand, "new-session") {
		t.Fatalf("leased attachment must not create a tmux session: %q", replacementCommand)
	}
}

func TestUnknownTerminalOperations_DoNotUseFallbackDriver(t *testing.T) {
	called := false
	withTerminalDriverRegistry(t, []terminalDriver{
		fakeTerminalDriver{
			id: "terminal-a",
			focusTitleFunc: func(string) error {
				called = true
				return nil
			},
			openAttachmentFunc: func(string) error {
				called = true
				return nil
			},
		},
	})
	target := Target{
		Session:    "repo/wt",
		TmuxPath:   "/opt/homebrew/bin/tmux",
		TmuxSocket: "/tmp/sock",
		TermBundle: "unknown-terminal",
	}

	if err := selectTab(target); err == nil {
		t.Fatal("selectTab should reject an unknown terminal")
	}
	if err := openTmuxSession(target); err == nil {
		t.Fatal("openTmuxSession should reject an unknown terminal")
	}
	if called {
		t.Fatal("unknown terminal operations should not call a fallback driver")
	}
}

func TestFocusTarget_TmuxSwitchFailureFallsBackToCapturedTitle(t *testing.T) {
	var titles []string
	withTerminalDriverRegistry(t, []terminalDriver{
		fakeTerminalDriver{
			id: "terminal-a",
			focusTitleFunc: func(title string) error {
				titles = append(titles, title)
				return nil
			},
		},
	})
	originalRun, originalOutput, originalLock := runCmd, runCmdOutput, withTmuxAttachmentLock
	t.Cleanup(func() {
		runCmd, runCmdOutput = originalRun, originalOutput
		withTmuxAttachmentLock = originalLock
	})
	withTmuxAttachmentLock = func(_ string, action tmuxAttachmentAction) error {
		return action()
	}
	runCmd = func(_ string, args ...string) error {
		if strings.Contains(strings.Join(args, " "), "switch-client") {
			return errors.New("client detached")
		}
		return nil
	}
	runCmdOutput = func(_ string, args ...string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "list-clients") {
			return []byte("100\tclient-a\t/dev/ttys007\txterm-example\n"), nil
		}
		return nil, nil
	}

	err := focusTarget(Target{
		Session:    "repo/wt",
		TmuxPath:   "/opt/homebrew/bin/tmux",
		TmuxSocket: "/tmp/sock",
		TermBundle: "terminal-a",
	})
	if err != nil {
		t.Fatalf("focusTarget returned error: %v", err)
	}
	if got := strings.Join(titles, ","); got != "repo/wt" {
		t.Fatalf("focusTitle calls = %q, want captured session title", got)
	}
}

func TestFocusTmuxSessionWithoutClient_ConcurrentCallsOpenOneAttachment(t *testing.T) {
	var mu sync.Mutex
	attached := false
	openCalls := 0
	switchCalls := 0
	leaseValue := ""
	requestValue := ""
	withTerminalDriverRegistry(t, []terminalDriver{
		fakeTerminalDriver{
			id: "terminal-a",
			openAttachmentFunc: func(string) error {
				mu.Lock()
				openCalls++
				mu.Unlock()
				go func() {
					time.Sleep(100 * time.Millisecond)
					mu.Lock()
					attached = true
					mu.Unlock()
				}()
				return nil
			},
		},
	})

	originalRun, originalOutput, originalLock := runCmd, runCmdOutput, withTmuxAttachmentLock
	t.Cleanup(func() {
		runCmd, runCmdOutput = originalRun, originalOutput
		withTmuxAttachmentLock = originalLock
	})
	lockPath := filepath.Join(t.TempDir(), "focus.lock")
	withTmuxAttachmentLock = func(_ string, action tmuxAttachmentAction) error {
		return lockTmuxAttachmentAt(lockPath, tmuxAttachmentLockTimeout, tmuxAttachmentPollInterval, action)
	}
	runCmd = func(_ string, args ...string) error {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "switch-client") {
			mu.Lock()
			switchCalls++
			mu.Unlock()
		}
		if strings.Contains(joined, "set-option") {
			mu.Lock()
			switch {
			case strings.Contains(joined, " -u ") && args[len(args)-1] == tmuxAttachmentLeaseOption:
				leaseValue = ""
			case len(args) >= 2 && args[len(args)-2] == tmuxAttachmentLeaseOption:
				leaseValue = args[len(args)-1]
			case len(args) >= 2 && args[len(args)-2] == tmuxAttachmentTargetOption:
				requestValue = args[len(args)-1]
			}
			mu.Unlock()
		}
		return nil
	}
	runCmdOutput = func(_ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "list-clients") {
			mu.Lock()
			defer mu.Unlock()
			if attached {
				return []byte("100\tclient-a\t/dev/ttys007\txterm-example\n"), nil
			}
			return nil, nil
		}
		if strings.Contains(joined, "show-options") {
			mu.Lock()
			defer mu.Unlock()
			if args[len(args)-1] == tmuxAttachmentTargetOption {
				return []byte(requestValue), nil
			}
			return []byte(leaseValue), nil
		}
		if strings.Contains(joined, "#{pane_current_path}") {
			return []byte("/worktrees/repo/wt\n"), nil
		}
		return nil, nil
	}

	target := Target{
		Session:    "repo/wt",
		TmuxPath:   "/opt/homebrew/bin/tmux",
		TmuxSocket: filepath.Join(t.TempDir(), "tmux.sock"),
		TermBundle: "terminal-a",
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := focusTmuxSession(target)
			errs <- err
		}()
	}
	close(start)
	for range 2 {
		select {
		case err := <-errs:
			if err != nil && !errors.Is(err, errTmuxFocusSuperseded) {
				t.Fatalf("focusTmuxSession returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent focus calls did not finish")
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if openCalls != 1 {
		t.Fatalf("openAttachment calls = %d, want exactly one", openCalls)
	}
	if switchCalls != 1 {
		t.Fatalf("switch-client calls = %d, want only the newest click", switchCalls)
	}
}

func TestFocusTmuxSession_OpenErrorRetriesLockWithoutReopening(t *testing.T) {
	openErr := errors.New("terminal failed to open")
	openCalls := 0
	leaseValue := ""
	requestValue := ""
	withTerminalDriverRegistry(t, []terminalDriver{
		fakeTerminalDriver{
			id: "terminal-a",
			openAttachmentFunc: func(string) error {
				openCalls++
				return openErr
			},
		},
	})

	originalRun, originalOutput, originalLock := runCmd, runCmdOutput, withTmuxAttachmentLock
	t.Cleanup(func() {
		runCmd, runCmdOutput = originalRun, originalOutput
		withTmuxAttachmentLock = originalLock
	})
	lockCalls := 0
	withTmuxAttachmentLock = func(_ string, action tmuxAttachmentAction) error {
		lockCalls++
		if lockCalls == 2 {
			return errTmuxAttachmentLockTimeout
		}
		return action()
	}
	runCmd = func(_ string, args ...string) error {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "set-option") && strings.Contains(joined, " -u ") && args[len(args)-1] == tmuxAttachmentLeaseOption:
			leaseValue = ""
		case len(args) >= 2 && args[len(args)-2] == tmuxAttachmentLeaseOption:
			leaseValue = args[len(args)-1]
		case len(args) >= 2 && args[len(args)-2] == tmuxAttachmentTargetOption:
			requestValue = args[len(args)-1]
		}
		return nil
	}
	runCmdOutput = func(_ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "list-clients"):
			return nil, nil
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentLeaseOption:
			return []byte(leaseValue), nil
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentTargetOption:
			return []byte(requestValue), nil
		}
		return nil, nil
	}

	_, err := focusTmuxSession(Target{
		Session:    "repo/wt",
		TmuxPath:   "/opt/homebrew/bin/tmux",
		TmuxSocket: "/tmp/sock",
		TermBundle: "terminal-a",
	})
	if !errors.Is(err, openErr) {
		t.Fatalf("focusTmuxSession error = %v, want open error", err)
	}
	if openCalls != 1 {
		t.Fatalf("openAttachment calls = %d, want one", openCalls)
	}
	if lockCalls != 4 {
		t.Fatalf("attachment lock calls = %d, want prepare, timed-out clear, retry, ownership check", lockCalls)
	}
}

func TestFocusTmuxSession_StaleClientCannotClearNewLease(t *testing.T) {
	var stateMu sync.Mutex
	listCalls := 0
	leaseValue := ""
	requestValue := ""
	openCalls := 0
	attached := false
	switchAttempts := 0
	clearStarted := make(chan struct{})
	releaseClear := make(chan struct{})
	marked := make(chan struct{})
	var clearOnce sync.Once
	var markedOnce sync.Once

	withTerminalDriverRegistry(t, []terminalDriver{
		fakeTerminalDriver{
			id: "terminal-a",
			openAttachmentFunc: func(string) error {
				stateMu.Lock()
				openCalls++
				attached = true
				stateMu.Unlock()
				return nil
			},
		},
	})

	originalRun, originalOutput := runCmd, runCmdOutput
	originalLock, originalWait := withTmuxAttachmentLock, waitForAttachedTmuxClient
	t.Cleanup(func() {
		runCmd, runCmdOutput = originalRun, originalOutput
		withTmuxAttachmentLock, waitForAttachedTmuxClient = originalLock, originalWait
	})

	var attachmentMu sync.Mutex
	var secondCaller atomic.Bool
	secondLockAttempt := make(chan struct{})
	var secondLockOnce sync.Once
	withTmuxAttachmentLock = func(_ string, action tmuxAttachmentAction) error {
		if secondCaller.Load() {
			secondLockOnce.Do(func() { close(secondLockAttempt) })
		}
		attachmentMu.Lock()
		defer attachmentMu.Unlock()
		return action()
	}
	waitForAttachedTmuxClient = func(Target) tmuxClient {
		stateMu.Lock()
		defer stateMu.Unlock()
		if attached {
			return tmuxClient{name: "replacement-client"}
		}
		return tmuxClient{}
	}

	runCmd = func(_ string, args ...string) error {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "set-option") && strings.Contains(joined, " -u ") && args[len(args)-1] == tmuxAttachmentLeaseOption:
			clearOnce.Do(func() { close(clearStarted) })
			<-releaseClear
			stateMu.Lock()
			leaseValue = ""
			stateMu.Unlock()
		case strings.Contains(joined, "set-option") && len(args) >= 2 && args[len(args)-2] == tmuxAttachmentLeaseOption:
			stateMu.Lock()
			leaseValue = args[len(args)-1]
			stateMu.Unlock()
			markedOnce.Do(func() { close(marked) })
		case strings.Contains(joined, "set-option") && len(args) >= 2 && args[len(args)-2] == tmuxAttachmentTargetOption:
			stateMu.Lock()
			requestValue = args[len(args)-1]
			stateMu.Unlock()
		case strings.Contains(joined, "switch-client"):
			stateMu.Lock()
			switchAttempts++
			attempt := switchAttempts
			stateMu.Unlock()
			if attempt == 1 {
				return errors.New("stale client detached")
			}
		}
		return nil
	}
	runCmdOutput = func(_ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		stateMu.Lock()
		defer stateMu.Unlock()
		switch {
		case strings.Contains(joined, "list-clients"):
			listCalls++
			if listCalls == 1 {
				return []byte("100\tstale-client\t/dev/ttys007\txterm-example\n"), nil
			}
			if attached {
				return []byte("100\treplacement-client\t/dev/ttys008\txterm-example\n"), nil
			}
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentTargetOption:
			return []byte(requestValue), nil
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentLeaseOption:
			return []byte(leaseValue), nil
		}
		return nil, nil
	}

	target := Target{
		Session:    "repo/wt",
		TmuxPath:   "/opt/homebrew/bin/tmux",
		TmuxSocket: "/tmp/sock",
		TermBundle: "terminal-a",
	}
	firstResult := make(chan error, 1)
	go func() {
		_, err := focusTmuxSession(target)
		firstResult <- err
	}()
	select {
	case <-clearStarted:
	case <-time.After(time.Second):
		t.Fatal("first caller did not reach stale lease clear")
	}

	secondCaller.Store(true)
	secondResult := make(chan error, 1)
	go func() {
		_, err := focusTmuxSession(target)
		secondResult <- err
	}()
	select {
	case <-secondLockAttempt:
	case <-time.After(time.Second):
		close(releaseClear)
		t.Fatal("second caller did not attempt attachment lock")
	}

	markedBeforeRelease := false
	select {
	case <-marked:
		markedBeforeRelease = true
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseClear)

	if err := <-firstResult; err == nil {
		t.Fatal("stale client switch should fail")
	}
	if err := <-secondResult; err != nil {
		t.Fatalf("second focus call returned error: %v", err)
	}
	if markedBeforeRelease {
		t.Fatal("a new attachment lease was marked before the stale client's clear completed")
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	if openCalls != 1 {
		t.Fatalf("openAttachment calls = %d, want one replacement", openCalls)
	}
}

func TestFocusTarget_NewerRequestForegroundsLast(t *testing.T) {
	var stateMu sync.Mutex
	requestValue := ""
	var focusOrder []string
	var switchTargets []string
	firstForegroundStarted := make(chan struct{})
	releaseFirstForeground := make(chan struct{})
	var foregroundOnce sync.Once

	withTerminalDriverRegistry(t, []terminalDriver{
		fakeTerminalDriver{
			id: "terminal-a",
			focusClientFunc: func(target terminalClientTarget) error {
				if target.title == "repo/session-a" {
					foregroundOnce.Do(func() { close(firstForegroundStarted) })
					<-releaseFirstForeground
				}
				stateMu.Lock()
				focusOrder = append(focusOrder, target.title)
				stateMu.Unlock()
				return nil
			},
		},
	})

	originalRun, originalOutput, originalLock := runCmd, runCmdOutput, withTmuxAttachmentLock
	t.Cleanup(func() {
		runCmd, runCmdOutput = originalRun, originalOutput
		withTmuxAttachmentLock = originalLock
	})
	var attachmentMu sync.Mutex
	withTmuxAttachmentLock = func(_ string, action tmuxAttachmentAction) error {
		attachmentMu.Lock()
		defer attachmentMu.Unlock()
		return action()
	}
	runCmd = func(_ string, args ...string) error {
		joined := strings.Join(args, " ")
		stateMu.Lock()
		defer stateMu.Unlock()
		switch {
		case strings.Contains(joined, "set-option") && len(args) >= 2 && args[len(args)-2] == tmuxAttachmentTargetOption:
			requestValue = args[len(args)-1]
		case strings.Contains(joined, "switch-client"):
			for i, arg := range args {
				if arg == "-t" && i+1 < len(args) {
					switchTargets = append(switchTargets, args[i+1])
				}
			}
		}
		return nil
	}
	runCmdOutput = func(_ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "list-clients"):
			return []byte("100\tclient-a\t/dev/ttys001\txterm-example\n200\tclient-b\t/dev/ttys002\txterm-example\n"), nil
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentTargetOption:
			stateMu.Lock()
			defer stateMu.Unlock()
			return []byte(requestValue), nil
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentLeaseOption:
			return nil, nil
		case strings.Contains(joined, "#{pane_current_path}"):
			return []byte("/worktrees/repo/wt\n"), nil
		}
		return nil, nil
	}

	targetA := Target{
		Session:    "repo/session-a",
		TmuxPath:   "/opt/homebrew/bin/tmux",
		TmuxSocket: "/tmp/sock",
		TmuxClient: "client-a",
		TermBundle: "terminal-a",
	}
	targetB := targetA
	targetB.Session = "repo/session-b"
	targetB.TmuxClient = "client-b"

	firstResult := make(chan error, 1)
	go func() { firstResult <- focusTarget(targetA) }()
	select {
	case <-firstForegroundStarted:
	case <-time.After(time.Second):
		t.Fatal("first request did not start terminal foregrounding")
	}
	secondResult := make(chan error, 1)
	go func() { secondResult <- focusTarget(targetB) }()
	select {
	case err := <-secondResult:
		close(releaseFirstForeground)
		t.Fatalf("newer request completed before ordered foreground release: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirstForeground)

	for name, result := range map[string]<-chan error{"first": firstResult, "second": secondResult} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatalf("%s focus request returned error: %v", name, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s focus request did not finish", name)
		}
	}

	stateMu.Lock()
	defer stateMu.Unlock()
	if got := strings.Join(focusOrder, ","); got != "repo/session-a,repo/session-b" {
		t.Fatalf("terminal focus order = %q, want oldest then newest", got)
	}
	if got := strings.Join(switchTargets, ","); got != "=repo/session-a,=repo/session-b" {
		t.Fatalf("tmux switch order = %q, want oldest then newest", got)
	}
}

func TestFocusTmuxSession_NewerSessionSupersedesOpeningOwner(t *testing.T) {
	var stateMu sync.Mutex
	leaseValue := ""
	requestValue := ""
	attached := false
	openCalls := 0
	var switchTargets []string
	activationCalls := 0
	openStarted := make(chan struct{})
	newerRequestMarked := make(chan struct{})
	releaseOpen := make(chan struct{})
	var openOnce sync.Once
	var newerOnce sync.Once

	withTerminalDriverRegistry(t, []terminalDriver{
		fakeTerminalDriver{
			id: "terminal-a",
			openAttachmentFunc: func(string) error {
				stateMu.Lock()
				openCalls++
				stateMu.Unlock()
				openOnce.Do(func() { close(openStarted) })
				<-releaseOpen
				stateMu.Lock()
				attached = true
				stateMu.Unlock()
				return nil
			},
		},
	})

	originalRun, originalOutput, originalLock := runCmd, runCmdOutput, withTmuxAttachmentLock
	t.Cleanup(func() {
		runCmd, runCmdOutput = originalRun, originalOutput
		withTmuxAttachmentLock = originalLock
	})
	var attachmentMu sync.Mutex
	withTmuxAttachmentLock = func(_ string, action tmuxAttachmentAction) error {
		attachmentMu.Lock()
		defer attachmentMu.Unlock()
		return action()
	}
	runCmd = func(name string, args ...string) error {
		joined := strings.Join(args, " ")
		stateMu.Lock()
		defer stateMu.Unlock()
		if name == osascriptPath {
			activationCalls++
		}
		switch {
		case strings.Contains(joined, "set-option") && strings.Contains(joined, " -u ") && args[len(args)-1] == tmuxAttachmentLeaseOption:
			leaseValue = ""
		case strings.Contains(joined, "set-option") && len(args) >= 2 && args[len(args)-2] == tmuxAttachmentLeaseOption:
			leaseValue = args[len(args)-1]
		case strings.Contains(joined, "set-option") && len(args) >= 2 && args[len(args)-2] == tmuxAttachmentTargetOption:
			requestValue = args[len(args)-1]
			if strings.HasSuffix(requestValue, base64.RawURLEncoding.EncodeToString([]byte("repo/session-b"))) {
				newerOnce.Do(func() { close(newerRequestMarked) })
			}
		case strings.Contains(joined, "switch-client"):
			for i, arg := range args {
				if arg == "-t" && i+1 < len(args) {
					switchTargets = append(switchTargets, args[i+1])
				}
			}
		}
		return nil
	}
	runCmdOutput = func(_ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		stateMu.Lock()
		defer stateMu.Unlock()
		switch {
		case strings.Contains(joined, "list-clients"):
			if attached {
				return []byte("100\tclient-a\t/dev/ttys007\txterm-example\n"), nil
			}
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentTargetOption:
			return []byte(requestValue), nil
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentLeaseOption:
			return []byte(leaseValue), nil
		case strings.Contains(joined, "#{pane_current_path}"):
			return []byte("/worktrees/repo/wt\n"), nil
		}
		return nil, nil
	}

	targetA := Target{
		Session:    "repo/session-a",
		TmuxPath:   "/opt/homebrew/bin/tmux",
		TmuxSocket: "/tmp/sock",
		TermBundle: "terminal-a",
	}
	targetB := targetA
	targetB.Session = "repo/session-b"
	firstResult := make(chan error, 1)
	go func() {
		firstResult <- focusTarget(targetA)
	}()
	select {
	case <-openStarted:
	case <-time.After(time.Second):
		t.Fatal("first focus call did not start opening its attachment")
	}

	secondResult := make(chan error, 1)
	go func() {
		_, err := focusTmuxSession(targetB)
		secondResult <- err
	}()
	select {
	case <-newerRequestMarked:
	case <-time.After(time.Second):
		close(releaseOpen)
		t.Fatal("newer focus call did not hand off the in-flight opener")
	}
	close(releaseOpen)
	select {
	case err := <-secondResult:
		if err != nil {
			t.Fatalf("newer focus call returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("newer focus call did not finish")
	}
	select {
	case err := <-firstResult:
		if err != nil {
			t.Fatalf("superseded focus call returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("superseded focus call did not finish")
	}

	stateMu.Lock()
	defer stateMu.Unlock()
	if openCalls != 1 {
		t.Fatalf("openAttachment calls = %d, want one shared opener", openCalls)
	}
	if got := strings.Join(switchTargets, ","); got != "=repo/session-b" {
		t.Fatalf("switch targets = %q, want only the newer session", got)
	}
	if activationCalls != 0 {
		t.Fatalf("superseded focus activated a stale terminal %d times", activationCalls)
	}
}

func TestLockTmuxAttachmentAt_TimeoutDoesNotRunAction(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "focus.lock")
	lock, err := acquireTmuxAttachmentLock(lockPath, time.Second, time.Millisecond)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	defer lock.Close()

	called := false
	err = lockTmuxAttachmentAt(lockPath, 20*time.Millisecond, time.Millisecond, func() error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("lockTmuxAttachmentAt should time out while the lock is held")
	}
	if called {
		t.Fatal("attachment action ran without acquiring the lock")
	}
}

func TestTmuxAttachmentLockPathIsStableAndSocketScoped(t *testing.T) {
	want := tmuxAttachmentLockPath("/tmp/socket-a")
	t.Setenv("TMPDIR", t.TempDir())
	if got := tmuxAttachmentLockPath("/tmp/socket-a"); got != want {
		t.Fatalf("tmuxAttachmentLockPath() = %q after TMPDIR change, want %q", got, want)
	}
	if got := tmuxAttachmentLockPath("/tmp/socket-b"); got == want {
		t.Fatalf("different tmux sockets share lock path %q", got)
	}
}

func TestAppleScriptTerminalDriver_MissingCapabilitiesReturnUnsupported(t *testing.T) {
	driver := appleScriptTerminalDriver{id: "incomplete-terminal"}
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "client", run: func() error { return driver.focusClientExact(terminalClientTarget{}) }},
		{name: "title", run: func() error { return driver.focusTitle("repo/wt") }},
		{name: "attachment", run: func() error { return driver.openAttachment("tmux attach") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, errTerminalOperationUnsupported) {
				t.Fatalf("operation error = %v, want errTerminalOperationUnsupported", err)
			}
		})
	}
}

func TestRunSelectionScript_ErrorContract(t *testing.T) {
	commandErr := errors.New("osascript failed")
	tests := []struct {
		name    string
		output  string
		cmdErr  error
		wantErr error
	}{
		{name: "selected", output: selectedMarker + "\n"},
		{name: "not found", output: "not found\n", wantErr: errTerminalNotFound},
		{name: "command error", cmdErr: commandErr, wantErr: commandErr},
	}
	original := runCmdOutput
	t.Cleanup(func() {
		runCmdOutput = original
	})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCmdOutput = func(string, ...string) ([]byte, error) {
				return []byte(test.output), test.cmdErr
			}
			err := runSelectionScript("script")
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("runSelectionScript error = %v, want %v", err, test.wantErr)
			}
		})
	}
}
