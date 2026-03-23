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

	staleTimeout   = 2 * time.Minute
	cleanupTimeout = 30 * time.Minute
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
	CleanStaleSessions(repoName, worktreeDir)

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
	if (s == StatusBusy || s == StatusDone || s == StatusWait) && time.Since(ts) > staleTimeout {
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

// CleanStaleSessions removes session files older than 30 minutes.
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
		if time.Since(ss.Timestamp) > cleanupTimeout {
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

// SessionFileInfo holds a parsed session with its repo/worktree metadata.
type SessionFileInfo struct {
	RepoName    string
	WorktreeDir string
	Session     SessionStatus
}

// RemoveSessionFile removes a single session file.
func RemoveSessionFile(repoName, worktreeDir, sessionID string) error {
	path := filepath.Join(StatusDir(), repoName, worktreeDir, sessionID+".json")
	return os.Remove(path)
}

// ScanAllSessions walks ~/.willow/status/ and returns all parsed sessions.
func ScanAllSessions() ([]SessionFileInfo, error) {
	statusDir := StatusDir()
	var results []SessionFileInfo

	repos, err := os.ReadDir(statusDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, repo := range repos {
		if !repo.IsDir() {
			continue
		}
		repoName := repo.Name()
		wtEntries, err := os.ReadDir(filepath.Join(statusDir, repoName))
		if err != nil {
			continue
		}
		for _, wt := range wtEntries {
			if !wt.IsDir() {
				continue
			}
			wtDir := wt.Name()
			sessEntries, err := os.ReadDir(filepath.Join(statusDir, repoName, wtDir))
			if err != nil {
				continue
			}
			for _, se := range sessEntries {
				if se.IsDir() || !strings.HasSuffix(se.Name(), ".json") {
					continue
				}
				data, err := os.ReadFile(filepath.Join(statusDir, repoName, wtDir, se.Name()))
				if err != nil {
					continue
				}
				var ss SessionStatus
				if err := json.Unmarshal(data, &ss); err != nil {
					continue
				}
				results = append(results, SessionFileInfo{
					RepoName:    repoName,
					WorktreeDir: wtDir,
					Session:     ss,
				})
			}
		}
	}
	return results, nil
}

// CleanEmptyStatusDirs removes empty worktree/repo directories under StatusDir().
func CleanEmptyStatusDirs() error {
	statusDir := StatusDir()
	repos, err := os.ReadDir(statusDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, repo := range repos {
		if !repo.IsDir() {
			continue
		}
		repoPath := filepath.Join(statusDir, repo.Name())
		wtEntries, err := os.ReadDir(repoPath)
		if err != nil {
			continue
		}
		for _, wt := range wtEntries {
			if !wt.IsDir() {
				continue
			}
			wtPath := filepath.Join(repoPath, wt.Name())
			entries, err := os.ReadDir(wtPath)
			if err != nil {
				continue
			}
			if len(entries) == 0 {
				os.Remove(wtPath)
			}
		}
		// Re-check if repo dir is now empty
		wtEntries, err = os.ReadDir(repoPath)
		if err == nil && len(wtEntries) == 0 {
			os.Remove(repoPath)
		}
	}
	return nil
}

// RemoveStatusDir removes the entire session directory for a worktree.
func RemoveStatusDir(repoName, worktreeDir string) {
	dir := filepath.Join(StatusDir(), repoName, worktreeDir)
	os.RemoveAll(dir)
	// Also remove legacy single file if present
	os.Remove(filepath.Join(StatusDir(), repoName, worktreeDir+".json"))
}
