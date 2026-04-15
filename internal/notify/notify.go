package notify

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/gen2brain/beeep"
)

//go:embed icon.png
var iconData []byte

var (
	iconOnce sync.Once
	iconPath string
)

// ensureIcon extracts the embedded icon to ~/.willow/icon.png on first call.
func ensureIcon() string {
	iconOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		p := filepath.Join(home, ".willow", "icon.png")
		if _, err := os.Stat(p); err == nil {
			iconPath = p
			return
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return
		}
		if err := os.WriteFile(p, iconData, 0o644); err != nil {
			return
		}
		iconPath = p
	})
	return iconPath
}

// Send fires a desktop notification with the willow icon.
// Works on macOS (terminal-notifier or osascript) and Linux (notify-send).
func Send(title, body string) error {
	beeep.AppName = "willow"
	return beeep.Notify(title, body, ensureIcon())
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
