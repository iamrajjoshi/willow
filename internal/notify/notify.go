package notify

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// notifierTimeout bounds how long a spawned terminal-notifier may run. It's a
// backstop: terminal-notifier hands the notification to Notification Center and
// exits in well under a second, but a wedged Notification Center daemon can
// leave it blocked. The hook fires notifications inline, so an unbounded exec
// would stall the agent — kill it instead.
const notifierTimeout = 10 * time.Second

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

// Click describes what happens when the user clicks a notification.
//
//   - Execute is a shell command run on click (terminal-notifier `-execute`).
//   - Sender is a terminal bundle id; it sets the notification icon and, on
//     click, brings that app forward (terminal-notifier `-sender`).
//   - Group coalesces repeat notifications for the same target so they replace
//     rather than stack (terminal-notifier `-group`).
//
// Click actions require terminal-notifier. macOS `display notification`
// (osascript) has no click-action API: clicking it activates osascript's host
// (Script Editor), not the agent's terminal.
type Click struct {
	Execute string
	Sender  string
	Group   string
}

// SendWithClick fires a notification that focuses the agent's session on click.
// It uses terminal-notifier when available; otherwise it falls back to the
// plain, non-clickable Send so notifications still fire on a stock Mac.
func SendWithClick(title, body string, click *Click) error {
	if runtime.GOOS == "darwin" && click != nil && click.Execute != "" {
		if path, err := exec.LookPath("terminal-notifier"); err == nil {
			return sendTerminalNotifier(path, title, body, *click)
		}
	}
	return Send(title, body)
}

func sendDarwin(title, body string) error {
	script := fmt.Sprintf("display notification %q with title %q", body, title)
	return exec.Command("osascript", "-e", script).Run()
}

func sendTerminalNotifier(path, title, body string, c Click) error {
	args := []string{"-title", title, "-message", body, "-execute", c.Execute}
	if c.Sender != "" {
		args = append(args, "-sender", c.Sender)
	}
	if c.Group != "" {
		args = append(args, "-group", c.Group)
	}

	// The hook fires notifications inline, so don't block on the notifier:
	// start it, then reap in the background. CommandContext kills it if it
	// wedges past notifierTimeout (a stuck Notification Center daemon).
	ctx, cancel := context.WithTimeout(context.Background(), notifierTimeout)
	cmd := exec.CommandContext(ctx, path, args...)
	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}
	go func() {
		defer cancel()
		_ = cmd.Wait()
	}()
	return nil
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
