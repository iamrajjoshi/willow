package notify

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

const (
	// ClickDispatchTimeout bounds terminal-notifier and its fallback together.
	ClickDispatchTimeout = time.Second
	// Standalone osascript notifications use the same per-dispatch bound.
	darwinNotifierTimeout = time.Second
)

// Send fires a desktop notification.
// macOS: uses `osascript` (shipped on every Mac).
// Linux: uses `notify-send`.
// Notifications appear under the sender tool's identity in Notification
// Center — custom icons and app names require a signed .app bundle, which
// willow doesn't ship.
func Send(title, body string) error {
	return systemNotifier().send(title, body)
}

func (n notifier) send(title, body string) error {
	switch n.goos {
	case "darwin":
		return n.sendDarwin(title, body)
	case "linux":
		return n.sendLinux(title, body)
	default:
		return fmt.Errorf("notify.Send: unsupported platform %q", n.goos)
	}
}

// Click holds terminal-notifier grouping and optional click action settings.
type Click struct {
	Execute string // shell command run on click (-execute)
	Group   string // coalesces repeats for the same target (-group)
}

type notifier struct {
	goos     string
	lookPath func(string) (string, error)
	run      func(context.Context, string, ...string) error
}

func systemNotifier() notifier {
	return notifier{
		goos:     runtime.GOOS,
		lookPath: exec.LookPath,
		run: func(ctx context.Context, path string, args ...string) error {
			return exec.CommandContext(ctx, path, args...).Run()
		},
	}
}

// SendWithClick uses terminal-notifier for a grouped or clickable notification, else falls back to Send.
func SendWithClick(title, body string, click *Click) error {
	return systemNotifier().sendWithClick(title, body, click)
}

// SupportsClick reports whether clickable built-in notifications are available.
func SupportsClick() bool {
	return systemNotifier().supportsClick()
}

func (n notifier) supportsClick() bool {
	if n.goos != "darwin" {
		return false
	}
	_, err := n.lookPath("terminal-notifier")
	return err == nil
}

func (n notifier) sendWithClick(title, body string, click *Click) error {
	if n.goos == "darwin" && click != nil && (click.Execute != "" || click.Group != "") {
		if path, err := n.lookPath("terminal-notifier"); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), ClickDispatchTimeout)
			defer cancel()
			notifierErr := n.sendTerminalNotifier(ctx, path, title, body, *click)
			if notifierErr == nil {
				return nil
			}
			if fallbackErr := n.sendDarwinContext(ctx, title, body); fallbackErr != nil {
				return errors.Join(
					fmt.Errorf("notify.SendWithClick: terminal-notifier: %w", notifierErr),
					fmt.Errorf("notify.SendWithClick: fallback: %w", fallbackErr),
				)
			}
			return nil
		}
	}
	return n.send(title, body)
}

func (n notifier) sendDarwin(title, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), darwinNotifierTimeout)
	defer cancel()
	return n.sendDarwinContext(ctx, title, body)
}

func (n notifier) sendDarwinContext(ctx context.Context, title, body string) error {
	script := fmt.Sprintf("display notification %q with title %q", body, title)
	err := n.run(ctx, "osascript", "-e", script)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (n notifier) sendTerminalNotifier(ctx context.Context, path, title, body string, c Click) error {
	args := []string{"-title", title, "-message", body}
	if c.Execute != "" {
		args = append(args, "-execute", c.Execute)
	}
	if c.Group != "" {
		args = append(args, "-group", c.Group)
	}

	err := n.run(ctx, path, args...)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (n notifier) sendLinux(title, body string) error {
	bin, err := n.lookPath("notify-send")
	if err != nil {
		return fmt.Errorf("notify.Send: notify-send not found: %w", err)
	}
	return n.run(context.Background(), bin, "-a", "willow", title, body)
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
