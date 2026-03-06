package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Status string

const (
	StatusBusy    Status = "BUSY"
	StatusDone    Status = "DONE"
	StatusWait    Status = "WAIT"
	StatusIdle    Status = "IDLE"
	StatusOffline Status = "--"
)

type WorktreeStatus struct {
	Status    Status    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Worktree  string    `json:"worktree,omitempty"`
}

func StatusDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".willow", "status")
}

func statusFilePath(repoName, worktreeDir string) string {
	return filepath.Join(StatusDir(), repoName, worktreeDir+".json")
}

// ReadStatus reads the status file for a worktree.
// Returns StatusOffline if the file doesn't exist or can't be read.
func ReadStatus(repoName, worktreeDir string) *WorktreeStatus {
	path := statusFilePath(repoName, worktreeDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &WorktreeStatus{Status: StatusOffline}
		}
		return &WorktreeStatus{Status: StatusOffline}
	}

	var ws WorktreeStatus
	if err := json.Unmarshal(data, &ws); err != nil {
		return &WorktreeStatus{Status: StatusOffline}
	}

	// Consider stale BUSY/DONE (>5 minutes) as IDLE
	if (ws.Status == StatusBusy || ws.Status == StatusDone) && time.Since(ws.Timestamp) > 5*time.Minute {
		ws.Status = StatusIdle
	}

	return &ws
}

// StatusIcon returns the emoji icon for a status.
func StatusIcon(s Status) string {
	switch s {
	case StatusBusy:
		return "\U0001F916" // robot face
	case StatusDone:
		return "\u2705" // white check mark
	case StatusWait:
		return "\u23F3" // hourglass
	case StatusIdle:
		return "\U0001F7E1" // yellow circle
	default:
		return "  "
	}
}

// StatusLabel returns the display label for a status.
func StatusLabel(s Status) string {
	return string(s)
}

// TimeSince returns a human-readable time-ago string.
func TimeSince(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
