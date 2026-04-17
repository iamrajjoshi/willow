package claude

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func lastReadPath(repoName, worktreeDir string) string {
	return filepath.Join(StatusDir(), repoName, worktreeDir, ".lastread")
}

func lastReadTime(repoName, worktreeDir string) time.Time {
	data, err := os.ReadFile(lastReadPath(repoName, worktreeDir))
	if err != nil {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return time.Time{}
	}
	return t
}

// MarkRead writes the current time to the .lastread marker file.
func MarkRead(repoName, worktreeDir string) error {
	path := lastReadPath(repoName, worktreeDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// IsUnread returns true if any DONE session has a timestamp after the .lastread marker.
func IsUnread(repoName, worktreeDir string) bool {
	return UnreadCount(repoName, worktreeDir) > 0
}

// UnreadCount returns the number of DONE sessions whose timestamp is after .lastread.
func UnreadCount(repoName, worktreeDir string) int {
	return CountUnreadIn(repoName, worktreeDir, ReadAllSessions(repoName, worktreeDir))
}

func CountUnreadIn(repoName, worktreeDir string, sessions []*SessionStatus) int {
	lr := lastReadTime(repoName, worktreeDir)
	count := 0
	for _, ss := range sessions {
		if ss.Status == StatusDone && (lr.IsZero() || ss.Timestamp.After(lr)) {
			count++
		}
	}
	return count
}
