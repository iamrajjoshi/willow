package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Transition describes a status change for a worktree.
type Transition struct {
	Key        string // "repo/wtDir"
	FromStatus Status
	ToStatus   Status
}

// TransitionState tracks the previous status of each worktree for transition detection.
type TransitionState map[string]string // "repo/wtDir" -> status string

func loadTransitionState(path string) TransitionState {
	data, err := os.ReadFile(path)
	if err != nil {
		return make(TransitionState)
	}
	var state TransitionState
	if err := json.Unmarshal(data, &state); err != nil {
		return make(TransitionState)
	}
	return state
}

func saveTransitionState(state TransitionState, path string) {
	data, _ := json.Marshal(state)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	os.Rename(tmp, path)
}

// DetectTransitions compares current statuses against saved state at stateFile
// and returns transitions from BUSY to a non-BUSY status.
// Each caller should use its own stateFile to track independently.
func DetectTransitions(current map[string]Status, stateFile string) []Transition {
	prev := loadTransitionState(stateFile)
	var transitions []Transition

	newState := make(TransitionState)
	for key, status := range current {
		newState[key] = string(status)
		if prevStatus, ok := prev[key]; ok {
			if prevStatus == string(StatusBusy) && status != StatusBusy {
				transitions = append(transitions, Transition{
					Key:        key,
					FromStatus: StatusBusy,
					ToStatus:   status,
				})
			}
		}
	}

	saveTransitionState(newState, stateFile)
	return transitions
}

// TmuxStateFile returns the path to the tmux status bar's state file.
func TmuxStateFile() string {
	return filepath.Join(StatusDir(), "..", "tmux-states.json")
}

// NotifyStateFile returns the path to the notify daemon's state file.
func NotifyStateFile() string {
	return filepath.Join(StatusDir(), "..", "notify-states.json")
}
