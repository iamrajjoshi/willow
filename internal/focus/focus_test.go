package focus

import (
	"strings"
	"testing"
)

func TestSocketFromEnv(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"full tmux value", "/private/tmp/tmux-501/default,12345,0", "/private/tmp/tmux-501/default"},
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

func TestExecuteCommand_TmuxTarget(t *testing.T) {
	got := ExecuteCommand("/usr/local/bin/willow", Target{
		Session:    "repo/feature",
		TmuxSocket: "/tmp/sock",
		TermBundle: "com.googlecode.iterm2",
	})
	for _, want := range []string{
		"'/usr/local/bin/willow' focus",
		"--session 'repo/feature'",
		"--tmux-socket '/tmp/sock'",
		"--term-bundle 'com.googlecode.iterm2'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ExecuteCommand() = %q, missing %q", got, want)
		}
	}
}

func TestExecuteCommand_OmitsEmptyFlags(t *testing.T) {
	got := ExecuteCommand("/usr/local/bin/willow", Target{Session: "repo/feature"})
	if strings.Contains(got, "--tmux-socket") {
		t.Errorf("ExecuteCommand() should omit --tmux-socket when empty: %q", got)
	}
	if strings.Contains(got, "--term-bundle") {
		t.Errorf("ExecuteCommand() should omit --term-bundle when empty: %q", got)
	}
}

func TestExecuteCommand_QuotesAdversarialInput(t *testing.T) {
	got := ExecuteCommand("/usr/local/bin/willow", Target{
		Session: "repo/foo'; rm -rf ~ #",
	})
	// The single quote in the session must be escaped so it can't break out of
	// the quoted argument and inject a command.
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

func TestSelectScripts_EmbedTitle(t *testing.T) {
	if !strings.Contains(iterm2SelectScript("repo/wt"), `"repo/wt"`) {
		t.Error("iterm2SelectScript should embed the quoted title")
	}
	if !strings.Contains(terminalSelectScript("repo/wt"), `"repo/wt"`) {
		t.Error("terminalSelectScript should embed the quoted title")
	}
}

// recordedCmd captures one runCmd invocation.
type recordedCmd struct {
	name string
	args []string
}

// withRecordedCmds swaps the package command runners for recorders and restores
// them after fn. clientList is what list-clients returns.
func withRecordedCmds(t *testing.T, clientList string, fn func(record *[]recordedCmd)) {
	t.Helper()
	origRun, origOut := runCmd, runCmdOutput
	t.Cleanup(func() { runCmd, runCmdOutput = origRun, origOut })

	var recorded []recordedCmd
	runCmd = func(name string, args ...string) error {
		recorded = append(recorded, recordedCmd{name, args})
		return nil
	}
	runCmdOutput = func(name string, args ...string) ([]byte, error) {
		recorded = append(recorded, recordedCmd{name, args})
		return []byte(clientList), nil
	}
	fn(&recorded)
}

func cmdArgsContain(cmds []recordedCmd, name, substr string) bool {
	for _, c := range cmds {
		if c.name != name {
			continue
		}
		if strings.Contains(strings.Join(c.args, " "), substr) {
			return true
		}
	}
	return false
}

func TestFocusTarget_TmuxSwitchesEachClient(t *testing.T) {
	withRecordedCmds(t, "client-a\nclient-b\n", func(record *[]recordedCmd) {
		err := focusTarget(Target{
			Session:    "repo/wt",
			TmuxSocket: "/tmp/sock",
			TermBundle: bundleITerm2,
		})
		if err != nil {
			t.Fatalf("focusTarget returned error: %v", err)
		}
		switches := 0
		for _, c := range *record {
			if c.name == "tmux" && len(c.args) > 0 && contains(c.args, "switch-client") {
				switches++
			}
		}
		if switches != 2 {
			t.Errorf("expected 2 switch-client calls (one per client), got %d", switches)
		}
		if !cmdArgsContain(*record, "osascript", "activate") {
			t.Error("expected the host terminal to be activated")
		}
		if cmdArgsContain(*record, "osascript", "iTerm2") {
			t.Error("tmux path should not also run the tab-select script")
		}
	})
}

func TestFocusTarget_NoTmuxSelectsTab(t *testing.T) {
	withRecordedCmds(t, "", func(record *[]recordedCmd) {
		_ = focusTarget(Target{Session: "repo/wt", TermBundle: bundleITerm2})
		if cmdArgsContain(*record, "tmux", "switch-client") {
			t.Error("no-tmux path should not switch tmux clients")
		}
		if !cmdArgsContain(*record, "osascript", "iTerm2") {
			t.Error("expected the iTerm2 tab-select script to run")
		}
		if !cmdArgsContain(*record, "osascript", "activate") {
			t.Error("expected the host terminal to be activated")
		}
	})
}

func TestFocusTarget_UnknownTerminalActivatesOnly(t *testing.T) {
	withRecordedCmds(t, "", func(record *[]recordedCmd) {
		_ = focusTarget(Target{Session: "repo/wt", TermBundle: "com.example.unknown"})
		for _, c := range *record {
			if c.name == "osascript" && strings.Contains(strings.Join(c.args, " "), "repeat with") {
				t.Error("unknown terminal should not run a tab-select script")
			}
		}
		if !cmdArgsContain(*record, "osascript", "activate") {
			t.Error("expected the host terminal to be activated")
		}
	})
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
