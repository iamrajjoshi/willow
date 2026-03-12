package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

type SessionStatus struct {
	Status    Status    `json:"status"`
	SessionID string    `json:"session_id"`
	Tool      string    `json:"tool,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Worktree  string    `json:"worktree,omitempty"`
}

func StatusDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".willow", "status")
}

// ReadAllSessions reads all session status files from the directory-based layout:
// ~/.willow/status/<repo>/<worktree>/*.json
func ReadAllSessions(repoName, worktreeDir string) []*SessionStatus {
	dir := filepath.Join(StatusDir(), repoName, worktreeDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var sessions []*SessionStatus
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var ss SessionStatus
		if err := json.Unmarshal(data, &ss); err != nil {
			continue
		}
		sessions = append(sessions, &ss)
	}
	return sessions
}

// ReadStatus reads the aggregate status for a worktree.
// It checks the new directory-based layout first, then falls back to the legacy
// single-file layout. Returns the highest-priority status across all sessions.
func ReadStatus(repoName, worktreeDir string) *WorktreeStatus {
	sessions := ReadAllSessions(repoName, worktreeDir)
	if len(sessions) > 0 {
		return aggregateStatus(sessions)
	}

	// Legacy single-file fallback
	return readLegacyStatus(repoName, worktreeDir)
}

func aggregateStatus(sessions []*SessionStatus) *WorktreeStatus {
	best := &WorktreeStatus{Status: StatusOffline}
	for _, ss := range sessions {
		effective := EffectiveStatus(ss.Status, ss.Timestamp)
		if statusPriority(effective) < statusPriority(best.Status) {
			best = &WorktreeStatus{
				Status:    effective,
				Timestamp: ss.Timestamp,
				Worktree:  ss.Worktree,
			}
		}
	}
	return best
}

func EffectiveStatus(s Status, ts time.Time) Status {
	if (s == StatusBusy || s == StatusDone || s == StatusWait) && time.Since(ts) > 5*time.Minute {
		return StatusIdle
	}
	return s
}

func statusPriority(s Status) int {
	switch s {
	case StatusBusy:
		return 0
	case StatusWait:
		return 1
	case StatusDone:
		return 2
	case StatusIdle:
		return 3
	default:
		return 4
	}
}

func readLegacyStatus(repoName, worktreeDir string) *WorktreeStatus {
	path := filepath.Join(StatusDir(), repoName, worktreeDir+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return &WorktreeStatus{Status: StatusOffline}
	}

	var ws WorktreeStatus
	if err := json.Unmarshal(data, &ws); err != nil {
		return &WorktreeStatus{Status: StatusOffline}
	}

	ws.Status = EffectiveStatus(ws.Status, ws.Timestamp)
	return &ws
}

// CleanStaleSessions removes session files older than 2 hours.
func CleanStaleSessions(repoName, worktreeDir string) {
	dir := filepath.Join(StatusDir(), repoName, worktreeDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var ss SessionStatus
		if err := json.Unmarshal(data, &ss); err != nil {
			os.Remove(filepath.Join(dir, e.Name()))
			continue
		}
		if time.Since(ss.Timestamp) > 2*time.Hour {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
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

// RemoveStatusDir removes the entire session directory for a worktree.
func RemoveStatusDir(repoName, worktreeDir string) {
	dir := filepath.Join(StatusDir(), repoName, worktreeDir)
	os.RemoveAll(dir)
	// Also remove legacy single file if present
	os.Remove(filepath.Join(StatusDir(), repoName, worktreeDir+".json"))
}
