package tmux

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
)

const defaultNotifyCommand = "afplay /System/Library/Sounds/Glass.aiff"

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
// worktree keys that transitioned from BUSY to a non-BUSY status.
func CheckTransitions(current map[string]claude.Status) []string {
	prev := loadState()
	var transitioned []string

	newState := make(StatusBarState)
	for key, status := range current {
		newState[key] = string(status)
		if prevStatus, ok := prev[key]; ok {
			if prevStatus == string(claude.StatusBusy) && status != claude.StatusBusy {
				transitioned = append(transitioned, key)
			}
		}
	}

	saveState(newState)
	return transitioned
}

// Notify runs the notification command (sound).
func Notify(command string) {
	if command == "" {
		command = defaultNotifyCommand
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Start()
}
