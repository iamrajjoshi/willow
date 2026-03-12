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

func NewSession(name, dir string) error {
	_, err := run("new-session", "-d", "-s", name, "-c", dir)
	return err
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
