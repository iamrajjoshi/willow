package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func InTmux() bool {
	return os.Getenv("TMUX") != ""
}

func run(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func SessionExists(name string) bool {
	err := exec.Command("tmux", "has-session", "-t", name).Run()
	return err == nil
}

func SendKeys(target string, keys ...string) error {
	args := append([]string{"send-keys", "-t", target}, keys...)
	_, err := run(args...)
	return err
}

// NewSession creates a tmux session and applies layout commands.
// Layout entries are raw tmux subcommands (e.g. "split-window -h").
// The session target (-t) and working directory (-c) are auto-injected.
// After layout setup, postWorktreeCreate commands are sent to every pane.
func NewSession(name, dir string, layout []string, postWorktreeCreate []string) error {
	if _, err := run("new-session", "-d", "-s", name, "-c", dir); err != nil {
		return err
	}

	for _, cmd := range layout {
		args := prepareLayoutCmd(strings.Fields(cmd), name, dir)
		run(args...)
	}

	for _, paneID := range listSessionPanes(name) {
		for _, cmd := range postWorktreeCreate {
			SendKeys(paneID, cmd, "Enter")
		}
	}

	return nil
}

func prepareLayoutCmd(args []string, session, dir string) []string {
	if len(args) == 0 {
		return args
	}

	subcmd := args[0]
	rest := args[1:]

	hasFlag := func(flag string) bool {
		for _, a := range rest {
			if a == flag {
				return true
			}
		}
		return false
	}

	if !hasFlag("-t") {
		rest = append([]string{"-t", session}, rest...)
	}

	if (subcmd == "split-window" || subcmd == "new-window") && !hasFlag("-c") {
		rest = append(rest, "-c", dir)
	}

	return append([]string{subcmd}, rest...)
}

func listSessionPanes(session string) []string {
	out, err := run("list-panes", "-t", session, "-s", "-F", "#{pane_id}")
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func KillSession(name string) error {
	_, err := run("kill-session", "-t", name)
	return err
}

func SwitchClient(name string) error {
	if InTmux() {
		_, err := run("switch-client", "-t", name)
		return err
	}
	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func CurrentSession() (string, error) {
	return run("display-message", "-p", "#{session_name}")
}

func CapturePane(target string) (string, error) {
	return run("capture-pane", "-ept", target, "-S", "-")
}

func ListSessions() ([]string, error) {
	out, err := run("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func SessionNameForWorktree(repoName, wtDirName string) string {
	return repoName + "/" + wtDirName
}
