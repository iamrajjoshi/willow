package tmux

import (
	"os/exec"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
)

const (
	defaultNotifyCommand     = "afplay /System/Library/Sounds/Glass.aiff"
	defaultNotifyWaitCommand = "afplay /System/Library/Sounds/Funk.aiff"
)

// CheckTransitions detects BUSY→non-BUSY transitions using the tmux state file.
func CheckTransitions(current map[string]claude.Status) []claude.Transition {
	return claude.DetectTransitions(current, claude.TmuxStateFile())
}

// NotifyWithContext sends sound notifications for transitions, skipping the current
// tmux session.
func NotifyWithContext(transitions []claude.Transition, cfg *config.Config) {
	currentSession, _ := CurrentSession()
	for _, t := range transitions {
		if t.Key == currentSession {
			continue
		}
		switch t.ToStatus {
		case claude.StatusWait:
			cmd := cfg.Tmux.NotifyWaitCommand
			if cmd == "" {
				cmd = defaultNotifyWaitCommand
			}
			Notify(cmd)
		case claude.StatusDone:
			Notify(cfg.Tmux.NotifyCommand)
		}
	}
}

// Notify runs the notification command (sound).
func Notify(command string) {
	if command == "" {
		command = defaultNotifyCommand
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Start()
}
