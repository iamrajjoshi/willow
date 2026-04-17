package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/iamrajjoshi/willow/internal/config"
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

// ListSessions returns the set of tmux session names, or an empty set if
// tmux isn't running.
func ListSessions() map[string]bool {
	out, err := run("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return map[string]bool{}
	}
	names := strings.Split(out, "\n")
	set := make(map[string]bool, len(names))
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			set[n] = true
		}
	}
	return set
}

func SendKeys(target string, keys ...string) error {
	args := append([]string{"send-keys", "-t", target}, keys...)
	_, err := run(args...)
	return err
}

// NewSession creates a tmux session and applies layout commands.
// Layout entries are raw tmux subcommands (e.g. "split-window -h").
// The session target (-t) and working directory (-c) are auto-injected.
// After layout setup, each pane receives its configured command (by index).
func NewSession(name, dir string, layout []string, panes []config.PaneConfig) error {
	if _, err := run("new-session", "-d", "-s", name, "-c", dir); err != nil {
		return err
	}

	for _, cmd := range layout {
		args := prepareLayoutCmd(strings.Fields(cmd), name, dir)
		if _, err := run(args...); err != nil {
			return fmt.Errorf("layout command %q: %w", cmd, err)
		}
	}

	paneIDs := listSessionPanes(name)
	for i, paneID := range paneIDs {
		if i < len(panes) && panes[i].Command != "" {
			if err := SendKeys(paneID, panes[i].Command, "Enter"); err != nil {
				return fmt.Errorf("pane %d command: %w", i, err)
			}
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

func SessionNameForWorktree(repoName, wtDirName string) string {
	return repoName + "/" + wtDirName
}
