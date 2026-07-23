package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	notifyKeyLockTimeout      = 1250 * time.Millisecond
	notifyKeyLockPollInterval = 10 * time.Millisecond
)

var errNotifyKeyLockTimeout = errors.New("timed out waiting for notification dispatch")

// Transition describes a status change for a worktree.
type Transition struct {
	Key        string // "repo/wtDir"
	FromStatus Status
	ToStatus   Status
	Refresh    bool // replaces a grouped clickable notification without a BUSY transition
}

// TransitionState tracks the previous status of each worktree for transition detection.
type TransitionState map[string]string // "repo/wtDir" -> status string

type notificationTransition struct {
	Status Status `json:"status"`
	Target string `json:"target,omitempty"`
}

type notificationTransitionState map[string]notificationTransition

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

// detectNotificationTransitions also replaces an existing grouped notification
// when the aggregate winner changes without passing through BUSY.
func detectNotificationTransitions(key string, status Status, target, stateFile string) []Transition {
	prev := loadNotificationTransitionState(stateFile)
	previous, exists := prev[key]

	var transitions []Transition
	if exists && status != StatusBusy {
		switch {
		case previous.Status == StatusBusy:
			transitions = append(transitions, Transition{
				Key:        key,
				FromStatus: previous.Status,
				ToStatus:   status,
			})
		case previous.Target != target:
			transitions = append(transitions, Transition{
				Key:        key,
				FromStatus: previous.Status,
				ToStatus:   status,
				Refresh:    true,
			})
		}
	}

	prev[key] = notificationTransition{Status: status, Target: target}
	saveNotificationTransitionState(prev, stateFile)
	return transitions
}

func loadNotificationTransitionState(path string) notificationTransitionState {
	data, err := os.ReadFile(path)
	if err != nil {
		return make(notificationTransitionState)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return make(notificationTransitionState)
	}
	state := make(notificationTransitionState, len(raw))
	for key, value := range raw {
		var legacyStatus Status
		if err := json.Unmarshal(value, &legacyStatus); err == nil {
			state[key] = notificationTransition{Status: legacyStatus}
			continue
		}
		var entry notificationTransition
		if err := json.Unmarshal(value, &entry); err == nil {
			state[key] = entry
		}
	}
	return state
}

func saveNotificationTransitionState(state notificationTransitionState, path string) {
	data, _ := json.Marshal(state)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	os.Rename(tmp, path)
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

func notifyKeyLockFile(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(StatusDir(), "..", "notify-"+hex.EncodeToString(sum[:])+".lock")
}

// withNotifyLock runs fn while holding an exclusive flock on the notify
// state lock file, serializing concurrent hooks that race on
// notify-states.json. Errors opening the lock file fall back to running
// fn unguarded — notifications are best-effort.
func withNotifyLock(fn func() error) error {
	return withNotifyFileLock(notifyLockFile(), fn)
}

// withNotifyKeyLock preserves dispatch order for one worktree without making
// unrelated worktrees wait for its external notification command.
func withNotifyKeyLock(key string, fn func() error) error {
	return withNotifyFileLockTimeout(notifyKeyLockFile(key), notifyKeyLockTimeout, notifyKeyLockPollInterval, fn)
}

func withNotifyFileLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fn()
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
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

func withNotifyFileLockTimeout(path string, timeout, interval time.Duration, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fn()
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fn()
	}
	defer f.Close()

	deadline := time.Now().Add(timeout)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			return fn()
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return fn()
		}
		if !time.Now().Before(deadline) {
			return errNotifyKeyLockTimeout
		}
		time.Sleep(interval)
	}
}
