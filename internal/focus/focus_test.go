package focus

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSocketFromEnv(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"full tmux value", "/private/tmp/tmux-501/default,12345,0", "/private/tmp/tmux-501/default"},
		{"socket containing comma", "/private/tmp/tmux/foo,bar.sock,12345,0", "/private/tmp/tmux/foo,bar.sock"},
		{"socket only", "/tmp/sock", "/tmp/sock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SocketFromEnv(tt.in); got != tt.want {
				t.Errorf("SocketFromEnv(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCanFocus(t *testing.T) {
	if !CanFocus(Target{TmuxPath: "/opt/homebrew/bin/tmux", TmuxSocket: "/tmp/sock"}) {
		t.Fatal("complete tmux target should be focusable")
	}
	for _, bundle := range []string{bundleITerm2, bundleTerminal, bundleGhostty} {
		if !CanFocus(Target{TermBundle: bundle}) {
			t.Fatalf("registered terminal %q should be focusable", bundle)
		}
	}
	if CanFocus(Target{TermBundle: "unsupported.terminal"}) {
		t.Fatal("unregistered terminal should not be focusable")
	}
	if CanFocus(Target{TmuxPath: "/opt/homebrew/bin/tmux"}) {
		t.Fatal("partial tmux context should not be focusable")
	}
}

func TestExecuteCommand_TmuxTarget(t *testing.T) {
	got := ExecuteCommand("/usr/local/bin/willow", Target{
		Session:    "repo/feature",
		TmuxPath:   "/opt/homebrew/bin/tmux",
		TmuxSocket: "/tmp/sock",
		TmuxPane:   "%7",
		TmuxClient: "/dev/ttys007",
		TermBundle: bundleITerm2,
	})
	for _, want := range []string{
		"'/usr/local/bin/willow' focus",
		"--session 'repo/feature'",
		"--tmux-path '/opt/homebrew/bin/tmux'",
		"--tmux-socket '/tmp/sock'",
		"--tmux-pane '%7'",
		"--tmux-client '/dev/ttys007'",
		"--term-bundle 'com.googlecode.iterm2'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ExecuteCommand() = %q, missing %q", got, want)
		}
	}
}

func TestExecuteCommand_OmitsEmptyFlags(t *testing.T) {
	got := ExecuteCommand("/usr/local/bin/willow", Target{Session: "repo/feature"})
	for _, flag := range []string{"--tmux-path", "--tmux-socket", "--tmux-pane", "--tmux-client", "--term-bundle"} {
		if strings.Contains(got, flag) {
			t.Errorf("ExecuteCommand() should omit %s when empty: %q", flag, got)
		}
	}
}

func TestExecuteCommand_QuotesAdversarialInput(t *testing.T) {
	got := ExecuteCommand("/usr/local/bin/willow", Target{Session: "repo/foo'; rm -rf ~ #"})
	if !strings.Contains(got, `'repo/foo'\''; rm -rf ~ #'`) {
		t.Errorf("ExecuteCommand() did not safely quote injection attempt: %q", got)
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"plain", "'plain'"},
		{"repo/wt", "'repo/wt'"},
		{"a'b", `'a'\''b'`},
	}
	for _, tt := range tests {
		if got := shellQuote(tt.in); got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSelectScripts_UseExactSupportedTitles(t *testing.T) {
	iterm := iterm2SelectScript("repo/wt")
	terminal := terminalSelectScript("repo/wt")
	ghostty := ghosttySelectScript("repo/wt")
	for name, script := range map[string]string{"iTerm2": iterm, "Terminal": terminal, "Ghostty": ghostty} {
		if !strings.Contains(script, `is equal to "repo/wt"`) {
			t.Errorf("%s script should use an exact title match:\n%s", name, script)
		}
		if strings.Contains(script, " contains ") {
			t.Errorf("%s script should not use substring matching:\n%s", name, script)
		}
	}
	if strings.Contains(terminal, "name of t") {
		t.Errorf("Terminal tabs do not expose a name property:\n%s", terminal)
	}
	if !strings.Contains(terminal, "custom title of t") {
		t.Errorf("Terminal script should read custom title:\n%s", terminal)
	}
}

func TestGhosttyWorkingDirectoryMarker(t *testing.T) {
	if got := ghosttyMarkerPath("/dev/ttys007"); got != "/tmp/willow-focus-ttys007" {
		t.Fatalf("ghosttyMarkerPath() = %q, want /tmp/willow-focus-ttys007", got)
	}
	got := string(terminalWorkingDirectorySequence("/tmp/willow focus"))
	want := "\x1b]7;file://localhost/tmp/willow%20focus\x07"
	if got != want {
		t.Fatalf("terminalWorkingDirectorySequence() = %q, want %q", got, want)
	}
	if got := string(terminalWorkingDirectorySequence("")); got != "\x1b]7;\x07" {
		t.Fatalf("terminalWorkingDirectorySequence(empty) = %q", got)
	}
	if !isGhosttyTerm("xterm-ghostty") || isGhosttyTerm("xterm-256color") {
		t.Fatal("isGhosttyTerm should recognize only Ghostty TERM values")
	}
}

func TestTerminalSelectScript_Compiles(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Terminal AppleScript is macOS-only")
	}
	for name, script := range map[string]string{
		"title": terminalSelectScript("repo/wt"),
		"tty":   terminalSelectTTYScript("/dev/ttys007"),
		"open":  terminalOpenScript("'/opt/homebrew/bin/tmux' -S '/tmp/sock' attach-session -t '=repo/wt'"),
	} {
		t.Run(name, func(t *testing.T) {
			out, err := exec.Command("/usr/bin/osacompile", "-o", "/dev/null", "-e", script).CombinedOutput()
			if err != nil {
				t.Fatalf("Terminal %s script does not compile: %v\n%s", name, err, out)
			}
		})
	}
}

func TestITermScripts_Compile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("iTerm2 AppleScript is macOS-only")
	}
	if _, err := os.Stat("/Applications/iTerm.app"); err != nil {
		if _, err := os.Stat("/Applications/iTerm2.app"); err != nil {
			t.Skip("iTerm2 is not installed")
		}
	}

	scripts := map[string]string{
		"select": iterm2SelectScript("repo/wt"),
		"open":   iterm2OpenScript("'/opt/homebrew/bin/tmux' -S '/tmp/sock' attach-session -t '=repo/wt'"),
		"tty":    iterm2SelectTTYScript("/dev/ttys007"),
	}
	for name, script := range scripts {
		t.Run(name, func(t *testing.T) {
			out, err := exec.Command("/usr/bin/osacompile", "-o", "/dev/null", "-e", script).CombinedOutput()
			if err != nil {
				t.Fatalf("iTerm2 %s script does not compile: %v\n%s", name, err, out)
			}
		})
	}
}

func TestGhosttyScripts_Compile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Ghostty AppleScript is macOS-only")
	}
	if _, err := os.Stat("/Applications/Ghostty.app/Contents/Resources/Ghostty.sdef"); err != nil {
		t.Skip("Ghostty 1.3+ is not installed")
	}

	scripts := map[string]string{
		"select":             ghosttySelectScript("repo/wt"),
		"tty":                ghosttySelectTTYScript("/dev/ttys007"),
		"working-directory":  ghosttySelectWorkingDirectoryScript("/tmp/willow-focus-ttys007"),
		"directory-snapshot": ghosttyWorkingDirectoriesScript(),
		"open":               ghosttyOpenScript("'/opt/homebrew/bin/tmux' -S '/tmp/sock' attach-session -t '=repo/wt'"),
	}
	for name, script := range scripts {
		t.Run(name, func(t *testing.T) {
			out, err := exec.Command("/usr/bin/osacompile", "-o", "/dev/null", "-e", script).CombinedOutput()
			if err != nil {
				t.Fatalf("Ghostty %s script does not compile: %v\n%s", name, err, out)
			}
		})
	}
}

type recordedCmd struct {
	name string
	args []string
}

type outputFunc func(name string, args ...string) ([]byte, error)

func withRecordedCmds(t *testing.T, output outputFunc, fn func(record *[]recordedCmd)) {
	t.Helper()
	origRun, origOut, origWriteSequence := runCmd, runCmdOutput, writeTerminalSequence
	origLock, origWait := withTmuxAttachmentLock, waitForAttachedTmuxClient
	t.Cleanup(func() {
		runCmd, runCmdOutput, writeTerminalSequence = origRun, origOut, origWriteSequence
		withTmuxAttachmentLock, waitForAttachedTmuxClient = origLock, origWait
	})

	var recorded []recordedCmd
	runCmd = func(name string, args ...string) error {
		recorded = append(recorded, recordedCmd{name, args})
		return nil
	}
	runCmdOutput = func(name string, args ...string) ([]byte, error) {
		recorded = append(recorded, recordedCmd{name, args})
		if output == nil {
			return nil, nil
		}
		return output(name, args...)
	}
	writeTerminalSequence = func(tty string, sequence []byte) error {
		recorded = append(recorded, recordedCmd{"terminal-sequence", []string{tty, string(sequence)}})
		return nil
	}
	withTmuxAttachmentLock = func(_ string, action tmuxAttachmentAction) error {
		return action()
	}
	waitForAttachedTmuxClient = func(Target) tmuxClient {
		return tmuxClient{}
	}
	fn(&recorded)
}

func trackRecordedTmuxFocusRequest() {
	recordedRun := runCmd
	recordedOutput := runCmdOutput
	requestValue := ""
	runCmd = func(name string, args ...string) error {
		if len(args) >= 2 && args[len(args)-2] == tmuxAttachmentTargetOption {
			requestValue = args[len(args)-1]
		}
		return recordedRun(name, args...)
	}
	runCmdOutput = func(name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[len(args)-1] == tmuxAttachmentTargetOption && requestValue != "" {
			if _, err := recordedOutput(name, args...); err != nil {
				return nil, err
			}
			return []byte(requestValue), nil
		}
		return recordedOutput(name, args...)
	}
}

func commandContains(cmds []recordedCmd, name string, parts ...string) bool {
	for _, cmd := range cmds {
		if cmd.name != name {
			continue
		}
		joined := strings.Join(cmd.args, " ")
		matched := true
		for _, part := range parts {
			if !strings.Contains(joined, part) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func tmuxOutput(session, clients string) outputFunc {
	return func(name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "#{pane_current_path}"):
			return []byte("/worktrees/repo/wt\n"), nil
		case strings.Contains(joined, "display-message"):
			return []byte(session + "\n"), nil
		case strings.Contains(joined, "list-clients"):
			return []byte(clients), nil
		case name == osascriptPath:
			return []byte(selectedMarker + "\n"), nil
		default:
			return nil, nil
		}
	}
}

func TestFocusTarget_TmuxPrefersCapturedClientAndSelectsPane(t *testing.T) {
	withRecordedCmds(t, tmuxOutput("repo/wt", "100\tclient-a\t/dev/ttys001\n200\tclient-b\t/dev/ttys002\n"), func(record *[]recordedCmd) {
		trackRecordedTmuxFocusRequest()
		err := focusTarget(Target{
			Session:    "repo/wt",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
			TmuxPane:   "%7",
			TmuxClient: "client-a",
			TermBundle: bundleITerm2,
		})
		if err != nil {
			t.Fatalf("focusTarget returned error: %v", err)
		}
		if !commandContains(*record, "/opt/homebrew/bin/tmux", "switch-client", "-c client-a", "-t =repo/wt") {
			t.Errorf("expected the captured client to switch exactly: %#v", *record)
		}
		if !commandContains(*record, "/opt/homebrew/bin/tmux", "set-option", "-s", "-q", "-u", tmuxAttachmentLeaseOption) {
			t.Errorf("expected an observed client to clear any stale attachment lease: %#v", *record)
		}
		if commandContains(*record, "/opt/homebrew/bin/tmux", "switch-client", "client-b") {
			t.Errorf("should not switch unrelated client-b: %#v", *record)
		}
		if !commandContains(*record, "/opt/homebrew/bin/tmux", "select-window", "-t %7") ||
			!commandContains(*record, "/opt/homebrew/bin/tmux", "select-pane", "-t %7") {
			t.Errorf("expected the agent pane to be selected: %#v", *record)
		}
		if !commandContains(*record, osascriptPath, "activate") {
			t.Error("expected the host terminal to be activated")
		}
		if !commandContains(*record, osascriptPath, "tty of s", "/dev/ttys001") {
			t.Error("expected the captured client's exact iTerm TTY to be selected")
		}
	})
}

func TestFocusTmuxSession_RetriesAttachmentLockTimeout(t *testing.T) {
	withRecordedCmds(t, tmuxOutput("repo/wt", "100\tclient-a\t/dev/ttys001\txterm-example\n"), func(_ *[]recordedCmd) {
		trackRecordedTmuxFocusRequest()
		uncontendedLock := withTmuxAttachmentLock
		lockCalls := 0
		withTmuxAttachmentLock = func(socket string, action tmuxAttachmentAction) error {
			lockCalls++
			if lockCalls == 1 {
				return errTmuxAttachmentLockTimeout
			}
			return uncontendedLock(socket, action)
		}

		client, err := focusTmuxSession(Target{
			Session:    "repo/wt",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
			TmuxClient: "client-a",
			TermBundle: bundleITerm2,
		})
		if err != nil {
			t.Fatalf("focusTmuxSession returned error: %v", err)
		}
		if client.name != "client-a" || lockCalls != 2 {
			t.Fatalf("client = %#v after %d lock calls, want one retry then client-a", client, lockCalls)
		}
	})
}

func TestFocusTarget_TmuxWithoutClientsOpensAttachment(t *testing.T) {
	withRecordedCmds(t, tmuxOutput("repo/wt", ""), func(record *[]recordedCmd) {
		err := focusTarget(Target{
			Session:    "repo/wt",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
			TermBundle: bundleTerminal,
		})
		if err != nil {
			t.Fatalf("focusTarget returned error: %v", err)
		}
		if !commandContains(*record, osascriptPath, "do script", "attach-session") ||
			commandContains(*record, osascriptPath, "attach-session", "'=repo/wt'") {
			t.Errorf("expected Terminal to open one target-independent attachment: %#v", *record)
		}
	})
}

func TestFocusTarget_TmuxWithoutClientsOpensITermAttachment(t *testing.T) {
	withRecordedCmds(t, tmuxOutput("repo/wt", ""), func(record *[]recordedCmd) {
		err := focusTarget(Target{
			Session:    "repo/wt",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
			TermBundle: bundleITerm2,
		})
		if err != nil {
			t.Fatalf("focusTarget returned error: %v", err)
		}
		if !commandContains(*record, osascriptPath, "create window with default profile command", "attach-session") ||
			commandContains(*record, osascriptPath, "attach-session", "'=repo/wt'") {
			t.Errorf("expected iTerm2 to open one target-independent attachment: %#v", *record)
		}
	})
}

func TestFocusTarget_TmuxWithoutClientsOpensGhosttyAttachment(t *testing.T) {
	withRecordedCmds(t, tmuxOutput("repo/wt", ""), func(record *[]recordedCmd) {
		err := focusTarget(Target{
			Session:    "repo/wt",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
			TermBundle: bundleGhostty,
		})
		if err != nil {
			t.Fatalf("focusTarget returned error: %v", err)
		}
		if !commandContains(*record, osascriptPath, "new surface configuration", "attach-session", "activate window") ||
			commandContains(*record, osascriptPath, "attach-session", "'=repo/wt'") {
			t.Errorf("expected Ghostty to open one target-independent attachment: %#v", *record)
		}
	})
}

func TestPrepareTmuxFocus_ActiveLeaseDoesNotClaimAnotherAttachment(t *testing.T) {
	request := newTmuxFocusRequest("repo/wt")
	leaseValue := fmt.Sprintf("%d:owner:nonce", time.Now().Add(time.Minute).UnixMilli())
	output := func(_ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[len(args)-1] == tmuxAttachmentTargetOption {
			return []byte(request.value), nil
		}
		if len(args) > 0 && args[len(args)-1] == tmuxAttachmentLeaseOption {
			return []byte(leaseValue), nil
		}
		return nil, nil
	}
	withRecordedCmds(t, output, func(record *[]recordedCmd) {
		preparation, err := prepareTmuxFocus(Target{
			Session:    "repo/wt",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
			TermBundle: bundleITerm2,
		}, "=repo/wt", request)
		if err != nil {
			t.Fatalf("prepareTmuxFocus returned error: %v", err)
		}
		if preparation.claimed {
			t.Fatal("active attachment lease should not be replaced")
		}
		if commandContains(*record, osascriptPath, "create window") {
			t.Fatalf("active attachment lease should suppress another terminal: %#v", *record)
		}
	})
}

func TestFocusTmuxSession_CurrentRequestWaitsForLateClient(t *testing.T) {
	leaseValue := fmt.Sprintf("%d:owner:nonce", time.Now().Add(time.Minute).UnixMilli())
	requestValue := ""
	clientAvailable := false
	output := func(_ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentTargetOption:
			return []byte(requestValue), nil
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentLeaseOption:
			return []byte(leaseValue), nil
		case strings.Contains(joined, "list-clients") && clientAvailable:
			return []byte("100\tclient-a\t/dev/ttys007\txterm-example\n"), nil
		}
		return nil, nil
	}
	withRecordedCmds(t, output, func(record *[]recordedCmd) {
		recordRun := runCmd
		runCmd = func(name string, args ...string) error {
			joined := strings.Join(args, " ")
			switch {
			case strings.Contains(joined, "set-option") && strings.Contains(joined, " -u ") && args[len(args)-1] == tmuxAttachmentLeaseOption:
				leaseValue = ""
			case len(args) >= 2 && args[len(args)-2] == tmuxAttachmentTargetOption:
				requestValue = args[len(args)-1]
			}
			return recordRun(name, args...)
		}
		waitCalls := 0
		waitForAttachedTmuxClient = func(Target) tmuxClient {
			waitCalls++
			if waitCalls == 1 {
				return tmuxClient{}
			}
			clientAvailable = true
			return tmuxClient{name: "client-a"}
		}

		client, err := focusTmuxSession(Target{
			Session:    "repo/wt",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
			TermBundle: bundleITerm2,
		})
		if err != nil {
			t.Fatalf("focusTmuxSession returned error: %v", err)
		}
		if client.name != "client-a" || waitCalls != 2 {
			t.Fatalf("late client = %#v after %d waits, want client-a after two", client, waitCalls)
		}
		if commandContains(*record, osascriptPath, "create window") {
			t.Fatalf("late client caused another terminal opener: %#v", *record)
		}
	})
}

func TestPrepareTmuxFocus_NewerTargetSupersedesDelayedAttachment(t *testing.T) {
	origRun, origOut, origLock := runCmd, runCmdOutput, withTmuxAttachmentLock
	t.Cleanup(func() {
		runCmd, runCmdOutput, withTmuxAttachmentLock = origRun, origOut, origLock
	})

	requestA := tmuxFocusRequest{value: "1:1:1:" + base64.RawURLEncoding.EncodeToString([]byte("repo/a")), startedAt: 1, pid: 1, nonce: 1, targetSession: "repo/a"}
	requestB := tmuxFocusRequest{value: "2:1:1:" + base64.RawURLEncoding.EncodeToString([]byte("repo/b")), startedAt: 2, pid: 1, nonce: 1, targetSession: "repo/b"}
	requestValue := requestA.value
	leaseValue := fmt.Sprintf("%d:owner:nonce", time.Now().Add(time.Minute).UnixMilli())
	delayedLease := leaseValue
	runCmd = func(_ string, args ...string) error {
		if len(args) >= 2 && args[len(args)-2] == tmuxAttachmentTargetOption {
			requestValue = args[len(args)-1]
		}
		if len(args) >= 2 && args[len(args)-2] == tmuxAttachmentLeaseOption {
			leaseValue = args[len(args)-1]
		}
		return nil
	}
	runCmdOutput = func(_ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentTargetOption:
			return []byte(requestValue), nil
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentLeaseOption:
			return []byte(leaseValue), nil
		case strings.Contains(joined, "list-clients"):
			return nil, nil
		default:
			return nil, nil
		}
	}
	withTmuxAttachmentLock = func(_ string, action tmuxAttachmentAction) error {
		return action()
	}

	preparation, err := prepareTmuxFocus(Target{
		Session:    "repo/b",
		TmuxPath:   "/opt/homebrew/bin/tmux",
		TmuxSocket: "/tmp/sock",
	}, "=repo/b", requestB)
	if err != nil {
		t.Fatalf("prepareTmuxFocus returned error: %v", err)
	}
	if preparation.claimed {
		t.Fatal("a newer click should hand off the existing opener, not claim another")
	}
	if leaseValue != delayedLease {
		t.Fatal("a newer click replaced the immutable opener lease")
	}
	storedRequest, err := readTmuxFocusRequest(Target{
		TmuxPath:   "/opt/homebrew/bin/tmux",
		TmuxSocket: "/tmp/sock",
	})
	if err != nil {
		t.Fatalf("readTmuxFocusRequest returned error: %v", err)
	}
	if storedRequest.targetSession != "repo/b" {
		t.Fatalf("handoff request targets %q, want repo/b", storedRequest.targetSession)
	}
}

func TestPrepareTmuxFocus_OlderInvocationCannotOverwriteNewerRequest(t *testing.T) {
	newer := tmuxFocusRequest{value: "2:1:1:" + base64.RawURLEncoding.EncodeToString([]byte("repo/b")), startedAt: 2, pid: 1, nonce: 1, targetSession: "repo/b"}
	older := tmuxFocusRequest{value: "1:1:1:" + base64.RawURLEncoding.EncodeToString([]byte("repo/a")), startedAt: 1, pid: 1, nonce: 1, targetSession: "repo/a"}
	output := func(_ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "list-clients"):
			return []byte("100\tclient-a\t/dev/ttys007\txterm-example\n"), nil
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentTargetOption:
			return []byte(newer.value), nil
		}
		return nil, nil
	}
	withRecordedCmds(t, output, func(record *[]recordedCmd) {
		_, err := prepareTmuxFocus(Target{
			Session:    "repo/a",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
		}, "=repo/a", older)
		if !errors.Is(err, errTmuxFocusSuperseded) {
			t.Fatalf("prepareTmuxFocus error = %v, want superseded", err)
		}
		if commandContains(*record, "/opt/homebrew/bin/tmux", "set-option") ||
			commandContains(*record, "/opt/homebrew/bin/tmux", "switch-client") {
			t.Fatalf("older invocation changed tmux state: %#v", *record)
		}
	})
}

func TestPrepareTmuxFocus_ListFailurePreservesDifferentTargetLease(t *testing.T) {
	origRun, origOut, origLock := runCmd, runCmdOutput, withTmuxAttachmentLock
	t.Cleanup(func() {
		runCmd, runCmdOutput, withTmuxAttachmentLock = origRun, origOut, origLock
	})

	requestA := tmuxFocusRequest{value: "1:1:1:" + base64.RawURLEncoding.EncodeToString([]byte("repo/a")), startedAt: 1, pid: 1, nonce: 1, targetSession: "repo/a"}
	requestB := tmuxFocusRequest{value: "2:1:1:" + base64.RawURLEncoding.EncodeToString([]byte("repo/b")), startedAt: 2, pid: 1, nonce: 1, targetSession: "repo/b"}
	requestValue := requestA.value
	leaseValue := fmt.Sprintf("%d:owner:nonce", time.Now().Add(time.Minute).UnixMilli())
	originalLease := leaseValue
	originalRequest := requestValue
	runCmd = func(_ string, args ...string) error {
		if len(args) >= 2 && args[len(args)-2] == tmuxAttachmentTargetOption {
			requestValue = args[len(args)-1]
		}
		if len(args) >= 2 && args[len(args)-2] == tmuxAttachmentLeaseOption {
			leaseValue = args[len(args)-1]
		}
		return nil
	}
	runCmdOutput = func(_ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentTargetOption:
			return []byte(requestValue), nil
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentLeaseOption:
			return []byte(leaseValue), nil
		case strings.Contains(joined, "list-clients"):
			return nil, fmt.Errorf("list clients failed")
		default:
			return nil, nil
		}
	}
	withTmuxAttachmentLock = func(_ string, action tmuxAttachmentAction) error {
		return action()
	}

	_, err := prepareTmuxFocus(Target{
		Session:    "repo/b",
		TmuxPath:   "/opt/homebrew/bin/tmux",
		TmuxSocket: "/tmp/sock",
	}, "=repo/b", requestB)
	if err == nil {
		t.Fatal("prepareTmuxFocus should return the list-clients failure")
	}
	if leaseValue != originalLease {
		t.Fatalf("list failure replaced the active lease: got %q, want %q", leaseValue, originalLease)
	}
	if requestValue != originalRequest {
		t.Fatalf("list failure replaced the target request: got %q, want %q", requestValue, originalRequest)
	}
}

func TestFocusTmuxSessionWithoutClient_ExpiredLeaseOpensAttachment(t *testing.T) {
	leaseValue := fmt.Sprintf("%d:owner:nonce", time.Now().Add(50*time.Millisecond).UnixMilli())
	requestValue := ""
	clientAvailable := false
	output := func(_ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentTargetOption:
			return []byte(requestValue), nil
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentLeaseOption:
			return []byte(leaseValue), nil
		case strings.Contains(joined, "list-clients") && clientAvailable:
			return []byte("100\tclient-a\t/dev/ttys007\txterm-example\n"), nil
		}
		return nil, nil
	}
	withRecordedCmds(t, output, func(record *[]recordedCmd) {
		recordRun := runCmd
		runCmd = func(name string, args ...string) error {
			joined := strings.Join(args, " ")
			switch {
			case strings.Contains(joined, "set-option") && strings.Contains(joined, " -u ") && args[len(args)-1] == tmuxAttachmentLeaseOption:
				leaseValue = ""
			case len(args) >= 2 && args[len(args)-2] == tmuxAttachmentLeaseOption:
				leaseValue = args[len(args)-1]
			case len(args) >= 2 && args[len(args)-2] == tmuxAttachmentTargetOption:
				requestValue = args[len(args)-1]
			}
			return recordRun(name, args...)
		}
		waitCalls := 0
		waitForAttachedTmuxClient = func(Target) tmuxClient {
			waitCalls++
			if waitCalls == 1 {
				time.Sleep(60 * time.Millisecond)
				return tmuxClient{}
			}
			clientAvailable = true
			return tmuxClient{name: "client-a"}
		}
		_, err := focusTmuxSession(Target{
			Session:    "repo/wt",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
			TermBundle: bundleITerm2,
		})
		if err != nil {
			t.Fatalf("focusTmuxSession returned error: %v", err)
		}
		if !commandContains(*record, osascriptPath, "create window") {
			t.Fatalf("an expired attachment lease should allow another terminal: %#v", *record)
		}
	})
}

func TestFocusTmuxSession_RetriesWhenPolledClientDetachesBeforeSwitch(t *testing.T) {
	leaseValue := fmt.Sprintf("%d:owner:nonce", time.Now().Add(30*time.Millisecond).UnixMilli())
	requestValue := ""
	clientAvailable := false
	output := func(_ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentTargetOption:
			return []byte(requestValue), nil
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentLeaseOption:
			return []byte(leaseValue), nil
		case strings.Contains(joined, "list-clients") && clientAvailable:
			return []byte("100\tclient-a\t/dev/ttys007\txterm-example\n"), nil
		}
		return nil, nil
	}
	withRecordedCmds(t, output, func(record *[]recordedCmd) {
		recordRun := runCmd
		runCmd = func(name string, args ...string) error {
			joined := strings.Join(args, " ")
			switch {
			case strings.Contains(joined, "set-option") && strings.Contains(joined, " -u ") && args[len(args)-1] == tmuxAttachmentLeaseOption:
				leaseValue = ""
			case len(args) >= 2 && args[len(args)-2] == tmuxAttachmentLeaseOption:
				leaseValue = args[len(args)-1]
			case len(args) >= 2 && args[len(args)-2] == tmuxAttachmentTargetOption:
				requestValue = args[len(args)-1]
			}
			return recordRun(name, args...)
		}
		waitCalls := 0
		waitForAttachedTmuxClient = func(Target) tmuxClient {
			waitCalls++
			if waitCalls == 1 {
				time.Sleep(40 * time.Millisecond)
				return tmuxClient{name: "detached-before-switch"}
			}
			clientAvailable = true
			return tmuxClient{name: "client-a"}
		}
		_, err := focusTmuxSession(Target{
			Session:    "repo/wt",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
			TermBundle: bundleITerm2,
		})
		if err != nil {
			t.Fatalf("focusTmuxSession returned error: %v", err)
		}
		if !commandContains(*record, osascriptPath, "create window") {
			t.Fatalf("an expired lease should be retried after the polled client detaches: %#v", *record)
		}
	})
}

func TestFocusTmuxSessionWithoutClient_LateClientPreventsAttachment(t *testing.T) {
	listCalls := 0
	output := func(_ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "list-clients") {
			listCalls++
			if listCalls > 1 {
				return []byte("100\tclient-a\t/dev/ttys007\txterm-example\n"), nil
			}
		}
		return nil, nil
	}
	withRecordedCmds(t, output, func(record *[]recordedCmd) {
		_, err := focusTmuxSession(Target{
			Session:    "repo/wt",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
			TermBundle: bundleITerm2,
		})
		if err != nil {
			t.Fatalf("focusTmuxSession returned error: %v", err)
		}
		if commandContains(*record, osascriptPath, "create window") {
			t.Fatalf("a client that appeared before open should prevent another terminal: %#v", *record)
		}
		if !commandContains(*record, "/opt/homebrew/bin/tmux", "switch-client", "-c client-a", "-t =repo/wt") {
			t.Fatalf("late client should be switched to the target session: %#v", *record)
		}
	})
}

func TestClearOwnedTmuxAttachmentLeaseDoesNotClearReplacement(t *testing.T) {
	replacement := fmt.Sprintf("%d:replacement", time.Now().Add(time.Minute).UnixMilli())
	output := func(_ string, args ...string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "show-options") {
			return []byte(replacement), nil
		}
		return nil, nil
	}
	withRecordedCmds(t, output, func(record *[]recordedCmd) {
		err := clearOwnedTmuxAttachmentLease(Target{
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
		}, tmuxAttachmentLease{value: "owned"})
		if err != nil {
			t.Fatalf("clearOwnedTmuxAttachmentLease returned error: %v", err)
		}
		if commandContains(*record, "/opt/homebrew/bin/tmux", "set-option", "-u") {
			t.Fatalf("an old opener cleared a replacement lease: %#v", *record)
		}
	})
}

func TestFinishTmuxAttachmentSwitchesConcurrentClientToTarget(t *testing.T) {
	lease := tmuxAttachmentLease{value: "2000:owner"}
	request := newTmuxFocusRequest("repo/wt")
	output := func(_ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentTargetOption:
			return []byte(request.value), nil
		case strings.Contains(joined, "show-options") && args[len(args)-1] == tmuxAttachmentLeaseOption:
			return []byte(lease.value), nil
		case strings.Contains(joined, "list-clients"):
			return []byte("100\tconcurrent-client\t/dev/ttys007\txterm-example\n"), nil
		}
		return nil, nil
	}
	withRecordedCmds(t, output, func(record *[]recordedCmd) {
		client, err := finishTmuxAttachment(Target{
			Session:    "repo/wt",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
		}, "=repo/wt", lease, request)
		if err != nil {
			t.Fatalf("finishTmuxAttachment returned error: %v", err)
		}
		if client.name != "concurrent-client" {
			t.Fatalf("finished client = %q, want concurrent-client", client.name)
		}
		if !commandContains(*record, "/opt/homebrew/bin/tmux", "switch-client", "-c concurrent-client", "-t =repo/wt") {
			t.Fatalf("concurrent client was not switched to the owned target: %#v", *record)
		}
	})
}

func TestFinishTmuxAttachment_StaleRequestCannotSwitchClient(t *testing.T) {
	lease := tmuxAttachmentLease{value: "2000:owner"}
	stale := tmuxFocusRequest{value: "1:1:1:stale", startedAt: 1, pid: 1, nonce: 1}
	newer := tmuxFocusRequest{value: "2:1:1:newer", startedAt: 2, pid: 1, nonce: 1}
	output := func(_ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[len(args)-1] == tmuxAttachmentTargetOption {
			return []byte(newer.value), nil
		}
		if strings.Contains(strings.Join(args, " "), "list-clients") {
			return []byte("100\tconcurrent-client\t/dev/ttys007\txterm-example\n"), nil
		}
		return nil, nil
	}
	withRecordedCmds(t, output, func(record *[]recordedCmd) {
		_, err := finishTmuxAttachment(Target{
			Session:    "repo/a",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
		}, "=repo/a", lease, stale)
		if !errors.Is(err, errTmuxFocusSuperseded) {
			t.Fatalf("finishTmuxAttachment error = %v, want superseded", err)
		}
		if commandContains(*record, "/opt/homebrew/bin/tmux", "switch-client") ||
			commandContains(*record, "/opt/homebrew/bin/tmux", "set-option", "-u") {
			t.Fatalf("stale request changed the active client or lease: %#v", *record)
		}
	})
}

func TestTmuxAttachmentRequestIsActive_StaleWaiterDoesNotRetry(t *testing.T) {
	lease := tmuxAttachmentLease{value: "2000:owner"}
	stale := tmuxFocusRequest{value: "1:1:1:stale", startedAt: 1, pid: 1, nonce: 1}
	newer := tmuxFocusRequest{value: "2:1:1:newer", startedAt: 2, pid: 1, nonce: 1}
	output := func(_ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[len(args)-1] == tmuxAttachmentTargetOption {
			return []byte(newer.value), nil
		}
		return nil, nil
	}
	withRecordedCmds(t, output, func(record *[]recordedCmd) {
		_, err := tmuxAttachmentRequestIsActive(Target{
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
		}, lease, stale)
		if !errors.Is(err, errTmuxFocusSuperseded) {
			t.Fatalf("tmuxAttachmentRequestIsActive error = %v, want superseded", err)
		}
		if commandContains(*record, "/opt/homebrew/bin/tmux", "set-option") ||
			commandContains(*record, "/opt/homebrew/bin/tmux", "switch-client") {
			t.Fatalf("stale waiter changed tmux state: %#v", *record)
		}
	})
}

func TestFocusTarget_TmuxGhosttyFocusesCapturedClient(t *testing.T) {
	output := func(name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "#{pane_current_path}"):
			return []byte("/worktrees/repo/wt\n"), nil
		case strings.Contains(joined, "display-message"):
			return []byte("repo/wt\n"), nil
		case strings.Contains(joined, "list-clients"):
			return []byte("100\tclient-a\t/dev/ttys007\txterm-ghostty\n"), nil
		case name == osascriptPath && strings.Contains(joined, "NSJSONSerialization"):
			return []byte(`{"ghostty-id":"/previous/ghostty"}`), nil
		case name == osascriptPath && strings.Contains(joined, bundleGhostty) && strings.Contains(joined, "/tmp/willow-focus-ttys007"):
			return []byte("ghostty-id\n"), nil
		case name == osascriptPath:
			return []byte("not found\n"), nil
		default:
			return nil, nil
		}
	}
	withRecordedCmds(t, output, func(record *[]recordedCmd) {
		trackRecordedTmuxFocusRequest()
		err := focusTarget(Target{
			Session:    "repo/wt",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
			TmuxClient: "client-a",
			TermBundle: bundleITerm2,
		})
		if err != nil {
			t.Fatalf("focusTarget returned error: %v", err)
		}
		if !commandContains(*record, "terminal-sequence", "/dev/ttys007", "file://localhost/tmp/willow-focus-ttys007") {
			t.Errorf("expected a unique working-directory marker on the captured Ghostty client: %#v", *record)
		}
		if !commandContains(*record, osascriptPath, "tty of term is equal to", "/dev/ttys007") ||
			!commandContains(*record, osascriptPath, `working directory of term is equal to "/tmp/willow-focus-ttys007"`, "focus term") {
			t.Errorf("expected Ghostty TTY selection followed by its working-directory fallback: %#v", *record)
		}
		if !commandContains(*record, "terminal-sequence", "/dev/ttys007", "file://localhost/previous/ghostty") {
			t.Errorf("expected the previous Ghostty working directory to be restored: %#v", *record)
		}
	})
}

func TestFocusTmuxSession_PreservesPathWhitespace(t *testing.T) {
	output := func(_ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "#{pane_current_path}"):
			return []byte("/worktrees/repo/wt \n"), nil
		case strings.Contains(joined, "display-message"):
			return []byte("repo/wt\n"), nil
		case strings.Contains(joined, "list-clients"):
			return []byte("100\tclient-a\t/dev/ttys007\txterm-ghostty\n"), nil
		default:
			return nil, nil
		}
	}
	withRecordedCmds(t, output, func(_ *[]recordedCmd) {
		client, err := focusTmuxSession(Target{
			Session:    "repo/wt",
			TmuxPath:   "/opt/homebrew/bin/tmux",
			TmuxSocket: "/tmp/sock",
		})
		if err != nil {
			t.Fatalf("focusTmuxSession returned error: %v", err)
		}
		if client.currentPath != "/worktrees/repo/wt " {
			t.Fatalf("currentPath = %q, want trailing space preserved", client.currentPath)
		}
	})
}

func TestFocusTarget_NoTmuxSelectsExactTab(t *testing.T) {
	withRecordedCmds(t, tmuxOutput("", ""), func(record *[]recordedCmd) {
		err := focusTarget(Target{Session: "repo/wt", TermBundle: bundleITerm2})
		if err != nil {
			t.Fatalf("focusTarget returned error: %v", err)
		}
		if commandContains(*record, "/opt/homebrew/bin/tmux", "switch-client") {
			t.Error("no-tmux path should not switch tmux clients")
		}
		if !commandContains(*record, osascriptPath, "name of s is equal to") {
			t.Error("expected the iTerm2 exact-tab script")
		}
		if !commandContains(*record, osascriptPath, "activate") {
			t.Error("expected the host terminal to be activated")
		}
	})
}

func TestFocusTarget_NoTmuxSelectsExactGhosttyTerminal(t *testing.T) {
	withRecordedCmds(t, tmuxOutput("", ""), func(record *[]recordedCmd) {
		err := focusTarget(Target{Session: "repo/wt", TermBundle: bundleGhostty})
		if err != nil {
			t.Fatalf("focusTarget returned error: %v", err)
		}
		if !commandContains(*record, osascriptPath, `name of term is equal to "repo/wt"`, "focus term") {
			t.Errorf("expected the exact Ghostty terminal title to be focused: %#v", *record)
		}
	})
}

func TestFocusTarget_UnknownTerminalStillActivates(t *testing.T) {
	withRecordedCmds(t, nil, func(record *[]recordedCmd) {
		err := focusTarget(Target{Session: "repo/wt", TermBundle: "com.example.unknown"})
		if err == nil {
			t.Fatal("focusTarget should report unsupported tab selection")
		}
		if !commandContains(*record, osascriptPath, "activate") {
			t.Error("expected the host application to be activated")
		}
	})
}

func TestSelectTab_MissingExactTitle(t *testing.T) {
	withRecordedCmds(t, func(name string, args ...string) ([]byte, error) {
		if name != osascriptPath {
			return nil, fmt.Errorf("unexpected command %s", name)
		}
		return []byte("not found\n"), nil
	}, func(_ *[]recordedCmd) {
		if err := selectTab(Target{Session: "repo/foo", TermBundle: bundleTerminal}); err == nil {
			t.Fatal("selectTab should fail when only a prefix-colliding title exists")
		}
	})
}

func TestMostRecentClient(t *testing.T) {
	if got := mostRecentClient([]byte("100\tclient-a\n300\tclient-c\n200\tclient-b\n")); got != "client-c" {
		t.Fatalf("mostRecentClient() = %q, want client-c", got)
	}
	if got := mostRecentClient(nil); got != "" {
		t.Fatalf("mostRecentClient(nil) = %q, want empty", got)
	}
}

func TestPreferredClient(t *testing.T) {
	clients := []byte("100\tclient-a\t/dev/ttys001\txterm-ghostty\n300\tclient-c\t/dev/ttys003\txterm-256color\n200\tclient-b\t/dev/ttys002\txterm-256color\n")
	if got := preferredClient(clients, "client-a"); got.name != "client-a" || got.tty != "/dev/ttys001" || got.termName != "xterm-ghostty" {
		t.Fatalf("preferredClient() = %#v, want captured client-a and its TTY", got)
	}
	if got := preferredClient(clients, "detached-client"); got.name != "client-c" || got.tty != "/dev/ttys003" {
		t.Fatalf("preferredClient() fallback = %#v, want most-recent client-c and its TTY", got)
	}
}

func TestSelectTerminal_FindsClientAcrossTerminalApps(t *testing.T) {
	withRecordedCmds(t, func(name string, args ...string) ([]byte, error) {
		if name != osascriptPath {
			return nil, nil
		}
		script := strings.Join(args, " ")
		if strings.Contains(script, "NSJSONSerialization") {
			return []byte(`{"ghostty-id":""}`), nil
		}
		if strings.Contains(script, bundleGhostty) && strings.Contains(script, "working directory of term") {
			return []byte("ghostty-id\n"), nil
		}
		return []byte("not found\n"), nil
	}, func(record *[]recordedCmd) {
		if err := selectTerminal("/dev/ttys007", "repo/wt", bundleITerm2, "xterm-ghostty", "/worktrees/repo/wt"); err != nil {
			t.Fatalf("selectTerminal: %v", err)
		}
		if !commandContains(*record, osascriptPath, bundleITerm2, "/dev/ttys007") ||
			!commandContains(*record, osascriptPath, bundleTerminal, "/dev/ttys007") ||
			!commandContains(*record, osascriptPath, bundleGhostty, "/dev/ttys007", "tty of term") ||
			!commandContains(*record, osascriptPath, bundleGhostty, `working directory of term is equal to "/tmp/willow-focus-ttys007"`) {
			t.Fatalf("expected iTerm2 and Terminal TTY probes followed by Ghostty: %#v", *record)
		}
	})
}

func TestSelectTerminal_DoesNotUseGhosttyFallbackWithoutGhosttyIdentity(t *testing.T) {
	withRecordedCmds(t, func(name string, args ...string) ([]byte, error) {
		if name == osascriptPath && strings.Contains(strings.Join(args, " "), "name of term") {
			return []byte(selectedMarker + "\n"), nil
		}
		return []byte("not found\n"), nil
	}, func(record *[]recordedCmd) {
		err := selectTerminal("/dev/ttys007", "repo/wt", "com.example.terminal", "xterm-256color", "/worktrees/repo/wt")
		if err == nil {
			t.Fatal("selectTerminal should not use a Ghostty title match for a non-Ghostty client")
		}
		if commandContains(*record, "terminal-sequence") || commandContains(*record, osascriptPath, "name of term") {
			t.Fatalf("non-Ghostty client should not receive Ghostty fallbacks: %#v", *record)
		}
	})
}

func TestGhosttyTerminalIDByWorkingDirectory_RejectsNotFound(t *testing.T) {
	withRecordedCmds(t, func(_ string, _ ...string) ([]byte, error) {
		return []byte("not found\n"), nil
	}, func(_ *[]recordedCmd) {
		if got := ghosttyTerminalIDByWorkingDirectory("/tmp/willow-focus-ttys007"); got != "" {
			t.Fatalf("ghosttyTerminalIDByWorkingDirectory() = %q, want empty", got)
		}
	})
}

func TestTmuxAttachCommand_QuotesArguments(t *testing.T) {
	got := tmuxAttachCommand(Target{
		Session:    "repo/foo'; touch /tmp/pwned #",
		TmuxPath:   "/opt/homebrew/bin/tmux",
		TmuxSocket: "/tmp/socket with spaces",
	})
	for _, want := range []string{
		"'/opt/homebrew/bin/tmux'",
		"-S '/tmp/socket with spaces'",
		`-t '=repo/foo'\''; touch /tmp/pwned #'`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("tmuxAttachCommand() = %q, missing %q", got, want)
		}
	}
}
