package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
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
// and returns transitions from BUSY to a non-BUSY status. State for keys not
// in current is preserved so callers can update a subset of worktrees without
// losing track of the rest — the per-hook dispatch path relies on this to
// avoid clobbering sibling-worktree state.
func DetectTransitions(current map[string]Status, stateFile string) []Transition {
	prev := loadTransitionState(stateFile)
	var transitions []Transition

	newState := make(TransitionState, len(prev)+len(current))
	for key, status := range prev {
		newState[key] = status
	}
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

// notifyLockFile returns the path to the advisory lock file guarding
// concurrent read-modify-write on NotifyStateFile().
func notifyLockFile() string {
	return filepath.Join(StatusDir(), "..", "notify-states.lock")
}

// withNotifyLock runs fn while holding an exclusive flock on the notify
// state lock file, serializing concurrent hooks that race on
// notify-states.json. Errors opening the lock file fall back to running
// fn unguarded — notifications are best-effort.
func withNotifyLock(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(notifyLockFile()), 0o755); err != nil {
		return fn()
	}
	f, err := os.OpenFile(notifyLockFile(), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fn()
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fn()
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
