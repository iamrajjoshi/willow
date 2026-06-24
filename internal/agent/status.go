package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iamrajjoshi/willow/internal/agent/harness"
	"github.com/iamrajjoshi/willow/internal/config"
)

type Status string

const (
	StatusBusy    Status = "BUSY"
	StatusDone    Status = "DONE"
	StatusWait    Status = "WAIT"
	StatusIdle    Status = "IDLE"
	StatusOffline Status = "--"

	staleTimeout   = 2 * time.Minute
	shortSessionID = 8
)

type WorktreeStatus struct {
	Status    Status    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Worktree  string    `json:"worktree,omitempty"`
}

type SessionStatus struct {
	Harness        string    `json:"harness,omitempty"`
	SessionID      string    `json:"session_id"`
	Status         Status    `json:"status"`
	Timestamp      time.Time `json:"timestamp"`
	StartTime      time.Time `json:"start_time,omitempty"`
	Tool           string    `json:"tool,omitempty"`
	ToolCount      int       `json:"tool_count,omitempty"`
	Model          string    `json:"model,omitempty"`
	TurnID         string    `json:"turn_id,omitempty"`
	PermissionMode string    `json:"permission_mode,omitempty"`
	Worktree       string    `json:"worktree,omitempty"`
}

func StatusDir() string {
	return filepath.Join(config.WillowHome(), "status")
}

// ReadAllSessions reads all session status files from:
// <willow-base>/status/<repo>/<worktree>/<harness>/*.json.
func ReadAllSessions(repoName, worktreeDir string) []*SessionStatus {
	dir := StatusWorktreeDir(repoName, worktreeDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var sessions []*SessionStatus
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		harnessID := e.Name()
		sessions = append(sessions, readHarnessSessions(repoName, worktreeDir, harnessID)...)
	}
	return sessions
}

func readHarnessSessions(repoName, worktreeDir, harnessID string) []*SessionStatus {
	dir := SessionDir(repoName, worktreeDir, harnessID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var sessions []*SessionStatus
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		sessionID := strings.TrimSuffix(e.Name(), ".json")
		if ss := readSessionFile(repoName, worktreeDir, harnessID, sessionID, filepath.Join(dir, e.Name())); ss != nil {
			sessions = append(sessions, ss)
		}
	}
	return sessions
}

func readSessionFile(repoName, worktreeDir, harnessID, sessionID, path string) *SessionStatus {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var ss SessionStatus
	if err := json.Unmarshal(data, &ss); err != nil {
		_ = removeSessionArtifacts(repoName, worktreeDir, harnessID, sessionID)
		return nil
	}
	applySessionDefaults(&ss, harnessID, sessionID, worktreeDir)
	return &ss
}

func applySessionDefaults(ss *SessionStatus, harnessID, sessionID, worktreeDir string) {
	if ss.SessionID == "" {
		ss.SessionID = sessionID
	}
	if ss.Harness == "" {
		ss.Harness = harnessID
	}
	if ss.Harness == "" {
		ss.Harness = harness.ClaudeID
	}
	if ss.Worktree == "" {
		ss.Worktree = worktreeDir
	}
}

// ReadStatus reads the aggregate status for a worktree.
// Returns the highest-priority status across all sessions.
func ReadStatus(repoName, worktreeDir string) *WorktreeStatus {
	sessions := ReadAllSessions(repoName, worktreeDir)
	if len(sessions) > 0 {
		return AggregateStatus(sessions)
	}
	return &WorktreeStatus{Status: StatusOffline}
}

func AggregateStatus(sessions []*SessionStatus) *WorktreeStatus {
	best := &WorktreeStatus{Status: StatusOffline}
	for _, ss := range sessions {
		effective := EffectiveStatus(ss.Status, ss.Timestamp)
		if StatusOrder(effective) < StatusOrder(best.Status) {
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
	if (s == StatusBusy || s == StatusWait) && time.Since(ts) > staleTimeout {
		return StatusIdle
	}
	return s
}

// StatusOrder returns a sort-priority for statuses (lower = higher priority).
// BUSY(0) < WAIT(1) < DONE(2) < IDLE(3) < everything else(4).
func StatusOrder(s Status) int {
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

// IsActive reports whether a status represents an active agent session
// (BUSY, WAIT, or DONE but not yet dismissed).
func IsActive(s Status) bool {
	return s == StatusBusy || s == StatusWait || s == StatusDone
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

func ShortSessionID(sessionID string) string {
	if len(sessionID) <= shortSessionID {
		return sessionID
	}
	return sessionID[:shortSessionID]
}

// SessionFileInfo holds a parsed session with its repo/worktree metadata.
type SessionFileInfo struct {
	RepoName    string
	WorktreeDir string
	Session     SessionStatus
}

func RemoveSessionFileForSession(repoName, worktreeDir string, session SessionStatus) error {
	return removeSessionArtifacts(repoName, worktreeDir, session.Harness, session.SessionID)
}

func removeSessionArtifacts(repoName, worktreeDir, harnessID, sessionID string) error {
	if harnessID == "" || sessionID == "" {
		return nil
	}

	var firstErr error
	for _, path := range sessionArtifactPaths(repoName, worktreeDir, harnessID, sessionID) {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func sessionArtifactPaths(repoName, worktreeDir, harnessID, sessionID string) []string {
	return []string{
		SessionPath(repoName, worktreeDir, harnessID, sessionID),
		FilesPathForHarness(repoName, worktreeDir, harnessID, sessionID),
		TimelinePathForHarness(repoName, worktreeDir, harnessID, sessionID),
	}
}

// ScanAllSessions walks the willow status directory and returns all parsed sessions.
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
			wtPath := filepath.Join(statusDir, repoName, wtDir)
			sessEntries, err := os.ReadDir(wtPath)
			if err != nil {
				continue
			}
			for _, se := range sessEntries {
				if !se.IsDir() {
					continue
				}
				for _, ss := range readHarnessSessions(repoName, wtDir, se.Name()) {
					results = append(results, SessionFileInfo{
						RepoName:    repoName,
						WorktreeDir: wtDir,
						Session:     *ss,
					})
				}
			}
		}
	}
	return results, nil
}

// ReadFilesTouched reads the sidecar .files list for a session.
// Returns deduplicated file paths the agent has written/edited.
func ReadFilesTouched(repoName, worktreeDir, sessionID string) []string {
	path := findFilesPath(repoName, worktreeDir, sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var result []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !seen[line] {
			seen[line] = true
			result = append(result, line)
		}
	}
	return result
}

func findFilesPath(repoName, worktreeDir, sessionID string) string {
	dir := StatusWorktreeDir(repoName, worktreeDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return FilesPathForHarness(repoName, worktreeDir, harness.ClaudeID, sessionID)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := FilesPathForHarness(repoName, worktreeDir, e.Name(), sessionID)
		if fileExists(path) {
			return path
		}
	}
	return FilesPathForHarness(repoName, worktreeDir, harness.ClaudeID, sessionID)
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
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				harnessPath := filepath.Join(wtPath, e.Name())
				harnessEntries, err := os.ReadDir(harnessPath)
				if err == nil && len(harnessEntries) == 0 {
					os.Remove(harnessPath)
				}
			}
			entries, err = os.ReadDir(wtPath)
			if err != nil {
				continue
			}
			if len(entries) == 0 {
				os.Remove(wtPath)
			}
		}
		wtEntries, err = os.ReadDir(repoPath)
		if err == nil && len(wtEntries) == 0 {
			os.Remove(repoPath)
		}
	}
	return nil
}

func StatusWorktreeDir(repoName, worktreeDir string) string {
	return filepath.Join(StatusDir(), repoName, worktreeDir)
}

func SessionDir(repoName, worktreeDir, harnessID string) string {
	harnessID = harness.NormalizeID(harnessID)
	return filepath.Join(StatusWorktreeDir(repoName, worktreeDir), harnessID)
}

func SessionPath(repoName, worktreeDir, harnessID, sessionID string) string {
	return filepath.Join(SessionDir(repoName, worktreeDir, harnessID), sessionID+".json")
}

func FilesPathForHarness(repoName, worktreeDir, harnessID, sessionID string) string {
	return filepath.Join(SessionDir(repoName, worktreeDir, harnessID), sessionID+".files")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func MoveStatusDir(repoName, oldWorktreeDir, newWorktreeDir string) error {
	oldPath := StatusWorktreeDir(repoName, oldWorktreeDir)
	newPath := StatusWorktreeDir(repoName, newWorktreeDir)
	if oldPath == newPath {
		return nil
	}
	if _, err := os.Stat(oldPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

// RemoveStatusDir removes the entire session directory for a worktree.
func RemoveStatusDir(repoName, worktreeDir string) {
	os.RemoveAll(StatusWorktreeDir(repoName, worktreeDir))
}
