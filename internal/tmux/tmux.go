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

// NewSession creates a tmux session with windows/panes based on config layout.
// If no layout is configured, a single default window is created.
func NewSession(name, dir string, layout []config.WindowSpec) error {
	if len(layout) == 0 {
		_, err := run("new-session", "-d", "-s", name, "-c", dir)
		return err
	}

	// Create session with first window
	firstWin := layout[0]
	winName := firstWin.Name
	if winName == "" {
		winName = "main"
	}
	if _, err := run("new-session", "-d", "-s", name, "-n", winName, "-c", dir); err != nil {
		return err
	}

	// Add panes to first window
	createPanes(name, winName, dir, firstWin)

	// Create additional windows
	for _, spec := range layout[1:] {
		wName := spec.Name
		if wName == "" {
			wName = "window"
		}
		if _, err := run("new-window", "-t", name, "-n", wName, "-c", dir); err != nil {
			continue
		}
		createPanes(name, wName, dir, spec)
	}

	// Select first window
	run("select-window", "-t", name+":"+layout[0].Name)
	return nil
}

func createPanes(session, window, dir string, spec config.WindowSpec) {
	target := session + ":" + window
	panes := spec.Panes
	if panes <= 1 {
		return
	}
	for i := 1; i < panes; i++ {
		run("split-window", "-t", target, "-c", dir)
	}
	if spec.Layout != "" {
		run("select-layout", "-t", target, spec.Layout)
	}
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
