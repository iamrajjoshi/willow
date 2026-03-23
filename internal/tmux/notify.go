package tmux

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
)

const (
	defaultNotifyCommand     = "afplay /System/Library/Sounds/Glass.aiff"
	defaultNotifyWaitCommand = "afplay /System/Library/Sounds/Funk.aiff"
)

// Transition describes a status change for a worktree.
type Transition struct {
	Key        string        // "repo/wtDir"
	FromStatus claude.Status
	ToStatus   claude.Status
}

func stateFilePath() string {
	return filepath.Join(config.WillowHome(), "tmux-states.json")
}

// StatusBarState tracks the previous status of each worktree for transition detection.
type StatusBarState map[string]string // "repo/wtDir" -> status string

func loadState() StatusBarState {
	data, err := os.ReadFile(stateFilePath())
	if err != nil {
		return make(StatusBarState)
	}
	var state StatusBarState
	if err := json.Unmarshal(data, &state); err != nil {
		return make(StatusBarState)
	}
	return state
}

func saveState(state StatusBarState) {
	data, _ := json.Marshal(state)
	os.WriteFile(stateFilePath(), data, 0o644)
}

// CheckTransitions compares current statuses against saved state and returns
// typed transitions from BUSY to a non-BUSY status.
func CheckTransitions(current map[string]claude.Status) []Transition {
	prev := loadState()
	var transitions []Transition

	newState := make(StatusBarState)
	for key, status := range current {
		newState[key] = string(status)
		if prevStatus, ok := prev[key]; ok {
			if prevStatus == string(claude.StatusBusy) && status != claude.StatusBusy {
				transitions = append(transitions, Transition{
					Key:        key,
					FromStatus: claude.StatusBusy,
					ToStatus:   status,
				})
			}
		}
	}

	saveState(newState)
	return transitions
}

// NotifyWithContext sends notifications for transitions, skipping the current
// tmux session and using different sounds for WAIT vs DONE.
func NotifyWithContext(transitions []Transition, cfg *config.Config) {
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
