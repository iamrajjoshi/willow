// Package focus brings an agent's terminal session to the foreground when the
// user clicks a willow desktop notification.
//
// It shells out to tmux and osascript directly rather than importing
// internal/tmux: that package depends on internal/agent, and agent is what
// builds the click target, so importing tmux here would form a cycle.
package focus

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

const (
	bundleITerm2   = "com.googlecode.iterm2"
	bundleTerminal = "com.apple.Terminal"
)

// Indirected for tests. runCmd runs a command and discards output; runCmdOutput
// runs one and returns stdout.
var (
	runCmd = func(name string, args ...string) error {
		return exec.Command(name, args...).Run()
	}
	runCmdOutput = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).Output()
	}
)

// Target describes where a notification click should navigate. Every field is
// captured when the notification fires: the click handler runs later, detached,
// with an empty environment, so nothing can be read from env at click time.
//
// Session is "repo/wtDir". That string is simultaneously the tmux session name
// and the terminal tab title set by `shell-init --tab-title`, so it's all the
// click needs to find either target.
type Target struct {
	Session    string
	TmuxSocket string // tmux socket path; empty when the agent wasn't in tmux
	TermBundle string // host terminal bundle id, from __CFBundleIdentifier
}

// Focus brings the agent's session to the foreground. In tmux it points the
// attached clients at the session; otherwise it selects the matching terminal
// tab. Either way it then activates the host terminal app. Steps are
// best-effort: a failure in one doesn't abort the others.
func Focus(t Target) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("focus: unsupported platform %q", runtime.GOOS)
	}
	return focusTarget(t)
}

func focusTarget(t Target) error {
	var firstErr error
	if t.TmuxSocket != "" {
		if err := focusTmuxSession(t.TmuxSocket, t.Session); err != nil {
			firstErr = err
		}
	} else {
		selectTab(t)
	}

	if err := activate(t.TermBundle); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// focusTmuxSession points every attached client at the session. A bare
// switch-client has no "current client" when run detached (as the click
// handler is), so each client is switched by name.
func focusTmuxSession(socket, session string) error {
	if err := runCmd("tmux", "-S", socket, "has-session", "-t", session); err != nil {
		return fmt.Errorf("focus: tmux session %q not found: %w", session, err)
	}
	out, err := runCmdOutput("tmux", "-S", socket, "list-clients", "-F", "#{client_name}")
	if err != nil {
		return fmt.Errorf("focus: list tmux clients: %w", err)
	}
	for _, client := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if client = strings.TrimSpace(client); client == "" {
			continue
		}
		_ = runCmd("tmux", "-S", socket, "switch-client", "-c", client, "-t", session)
	}
	return nil
}

// activate brings the terminal app forward without opening a new window.
func activate(bundle string) error {
	if bundle == "" {
		return nil
	}
	script := fmt.Sprintf("tell application id %q to activate", bundle)
	return runCmd("osascript", "-e", script)
}

// selectTab tries to focus the terminal tab whose title matches the session
// name ("repo/wtDir", set by `shell-init --tab-title`). Only iTerm2 and
// Terminal.app expose tab control via AppleScript; on other terminals
// activate() alone brings the app forward. Best-effort: errors are swallowed
// because tab titles depend on the user's --tab-title setup.
func selectTab(t Target) {
	var script string
	switch t.TermBundle {
	case bundleITerm2:
		script = iterm2SelectScript(t.Session)
	case bundleTerminal:
		script = terminalSelectScript(t.Session)
	default:
		return
	}
	_ = runCmd("osascript", "-e", script)
}

func iterm2SelectScript(title string) string {
	return fmt.Sprintf(`tell application "iTerm2"
	repeat with w in windows
		repeat with t in tabs of w
			repeat with s in sessions of t
				if name of s contains %q then
					select w
					select t
					tell s to select
					return
				end if
			end repeat
		end repeat
	end repeat
end tell`, title)
}

func terminalSelectScript(title string) string {
	return fmt.Sprintf(`tell application "Terminal"
	repeat with w in windows
		repeat with t in tabs of w
			if (name of t contains %q) or (custom title of t contains %q) then
				set selected of t to true
				set frontmost of w to true
				return
			end if
		end repeat
	end repeat
end tell`, title, title)
}

// ExecuteCommand builds the shell command terminal-notifier runs on click.
// willowPath is the absolute willow binary, captured at fire time so the
// detached click handler doesn't depend on PATH.
func ExecuteCommand(willowPath string, t Target) string {
	parts := []string{shellQuote(willowPath), "focus", "--session", shellQuote(t.Session)}
	if t.TmuxSocket != "" {
		parts = append(parts, "--tmux-socket", shellQuote(t.TmuxSocket))
	}
	if t.TermBundle != "" {
		parts = append(parts, "--term-bundle", shellQuote(t.TermBundle))
	}
	return strings.Join(parts, " ")
}

// SocketFromEnv extracts the tmux socket path from a $TMUX value
// ("/path/to/socket,pid,session" → "/path/to/socket"). Empty if unset.
func SocketFromEnv(tmuxEnv string) string {
	if tmuxEnv == "" {
		return ""
	}
	socket, _, _ := strings.Cut(tmuxEnv, ",")
	return socket
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
