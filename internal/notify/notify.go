package notify

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Send fires a desktop notification.
// macOS: uses `osascript` (shipped on every Mac).
// Linux: uses `notify-send`.
// Notifications appear under the sender tool's identity in Notification
// Center — custom icons and app names require a signed .app bundle, which
// willow doesn't ship.
func Send(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		return sendDarwin(title, body)
	case "linux":
		return sendLinux(title, body)
	default:
		return fmt.Errorf("notify.Send: unsupported platform %q", runtime.GOOS)
	}
}

func sendDarwin(title, body string) error {
	script := fmt.Sprintf("display notification %q with title %q", body, title)
	return exec.Command("osascript", "-e", script).Run()
}

func sendLinux(title, body string) error {
	bin, err := exec.LookPath("notify-send")
	if err != nil {
		return fmt.Errorf("notify.Send: notify-send not found: %w", err)
	}
	return exec.Command(bin, "-a", "willow", title, body).Run()
}

// SendCustom runs a user-provided command with title/body available as
// WILLOW_NOTIFY_TITLE and WILLOW_NOTIFY_BODY env vars.
func SendCustom(command, title, body string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = append(cmd.Environ(),
		"WILLOW_NOTIFY_TITLE="+title,
		"WILLOW_NOTIFY_BODY="+body,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("custom notify command: %w: %s", err, out)
	}
	return nil
}
