package dashboard

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
)

func TestParseShortstatFromGit(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "full output",
			out:  " 3 files changed, 42 insertions(+), 12 deletions(-)",
			want: "3f +42 -12",
		},
		{
			name: "insertions only",
			out:  " 1 file changed, 10 insertions(+)",
			want: "1f +10",
		},
		{
			name: "deletions only",
			out:  " 2 files changed, 5 deletions(-)",
			want: "2f -5",
		},
		{
			name: "empty string",
			out:  "",
			want: "--",
		},
		{
			name: "single file single insertion and deletion",
			out:  " 1 file changed, 1 insertion(+), 1 deletion(-)",
			want: "1f +1 -1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := git.ParseShortstat(tt.out)
			if got != tt.want {
				t.Errorf("git.ParseShortstat(%q) = %q, want %q", tt.out, got, tt.want)
			}
		})
	}
}

func TestRenderNoSessions(t *testing.T) {
	out := render(nil, summary{Repos: 2, Agents: 0, Unread: 0}, 120, 0, false, 0, nil)
	if !strings.Contains(out, "no active sessions") {
		t.Errorf("expected 'no active sessions' empty-state copy, got:\n%s", out)
	}
	if !strings.Contains(out, "willow new") {
		t.Errorf("expected 'willow new' hint in empty state, got:\n%s", out)
	}
}

func TestRenderSingleSession(t *testing.T) {
	sessions := []session{
		{
			Repo:      "myrepo",
			Branch:    "feat--thing",
			SessionID: "abc12345rest",
			Status:    claude.StatusBusy,
			Tool:      "",
			DiffStats: "2f +10 -3",
			Age:       "5m",
			Unread:    false,
			WtDirName: "feat--thing",
		},
	}
	out := render(sessions, summary{Repos: 1, Agents: 1, Unread: 0}, 120, 0, false, 0, nil)

	if !strings.Contains(out, "myrepo") {
		t.Errorf("expected 'myrepo' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "feat--thing") {
		t.Errorf("expected 'feat--thing' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "2f +10 -3") {
		t.Errorf("expected diff stats in output, got:\n%s", out)
	}
	if !strings.Contains(out, "SESSION") {
		t.Errorf("expected SESSION column header, got:\n%s", out)
	}
	if !strings.Contains(out, "abc12345") {
		t.Errorf("expected shortened session id in output, got:\n%s", out)
	}
}

func TestRenderUnreadMarker(t *testing.T) {
	sessions := []session{
		{
			Repo:      "myrepo",
			Branch:    "feat--done",
			SessionID: "abc123",
			Status:    claude.StatusDone,
			DiffStats: "--",
			Age:       "2m",
			Unread:    true,
			WtDirName: "feat--done",
		},
	}
	out := render(sessions, summary{Repos: 1, Agents: 1, Unread: 1}, 120, 0, false, 0, nil)

	if !strings.Contains(out, "\u25CF") {
		t.Errorf("expected unread marker (bullet) in output, got:\n%s", out)
	}
}

func TestRenderBusyWithTool(t *testing.T) {
	sessions := []session{
		{
			Repo:      "myrepo",
			Branch:    "feat--edit",
			SessionID: "abc123",
			Status:    claude.StatusBusy,
			Tool:      "Edit",
			DiffStats: "1f +5",
			Age:       "1m",
			Unread:    false,
			WtDirName: "feat--edit",
		},
	}
	out := render(sessions, summary{Repos: 1, Agents: 1, Unread: 0}, 120, 0, false, 0, nil)

	if !strings.Contains(out, "(Edit)") {
		t.Errorf("expected '(Edit)' tool label in output, got:\n%s", out)
	}
}

func TestRenderSelectedRow(t *testing.T) {
	sessions := []session{
		{
			Repo:      "repo-a",
			Branch:    "branch-a",
			Status:    claude.StatusBusy,
			DiffStats: "--",
			Age:       "3m",
			WtDirName: "branch-a",
		},
		{
			Repo:      "repo-b",
			Branch:    "branch-b",
			Status:    claude.StatusDone,
			DiffStats: "1f +2",
			Age:       "8m",
			WtDirName: "branch-b",
		},
	}
	out := render(sessions, summary{Repos: 2, Agents: 2, Unread: 0}, 120, 1, false, 0, nil)

	if !strings.Contains(out, "\033[7m") {
		t.Errorf("expected inverse video escape sequence for selected row, got:\n%s", out)
	}
}

func TestRenderKeybindingHint(t *testing.T) {
	sessions := []session{
		{
			Repo:      "myrepo",
			Branch:    "main",
			Status:    claude.StatusBusy,
			DiffStats: "--",
			Age:       "1m",
			WtDirName: "main",
		},
	}
	out := render(sessions, summary{Repos: 1, Agents: 1, Unread: 0}, 120, 0, false, 0, nil)

	if !strings.Contains(out, "j/k: navigate") {
		t.Errorf("expected keybinding hint 'j/k: navigate' in output, got:\n%s", out)
	}
}

func TestRenderTimelineColumn(t *testing.T) {
	sparkline := strings.Repeat("\033[32m\u2588\033[0m", 30)
	sessions := []session{
		{
			Repo:      "myrepo",
			Branch:    "feat--thing",
			Status:    claude.StatusBusy,
			DiffStats: "--",
			Age:       "2m",
			WtDirName: "feat--thing",
			Timeline:  sparkline,
		},
	}

	// With timeline enabled
	out := render(sessions, summary{Repos: 1, Agents: 1, Unread: 0}, 200, 0, true, 0, nil)
	if !strings.Contains(out, "TIMELINE") {
		t.Errorf("expected 'TIMELINE' header when showTimeline=true, got:\n%s", out)
	}
	if !strings.Contains(out, "\u2588") {
		t.Errorf("expected sparkline blocks in output when showTimeline=true")
	}

	// With timeline disabled
	out = render(sessions, summary{Repos: 1, Agents: 1, Unread: 0}, 200, 0, false, 0, nil)
	if strings.Contains(out, "TIMELINE") {
		t.Errorf("expected no 'TIMELINE' header when showTimeline=false, got:\n%s", out)
	}
}

func TestRenderTimelineKeybindingHint(t *testing.T) {
	sessions := []session{
		{
			Repo:      "myrepo",
			Branch:    "main",
			Status:    claude.StatusBusy,
			DiffStats: "--",
			Age:       "1m",
			WtDirName: "main",
		},
	}
	out := render(sessions, summary{Repos: 1, Agents: 1, Unread: 0}, 120, 0, true, 0, nil)
	if !strings.Contains(out, "t: timeline") {
		t.Errorf("expected 't: timeline' in keybinding hint, got:\n%s", out)
	}
}

func runDashboardGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func setupDashboardRepo(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	src := filepath.Join(home, "src")
	bareDir := filepath.Join(config.ReposDir(), "dashrepo.git")
	wtPath := filepath.Join(config.WorktreesDir(), "dashrepo", "main")

	if err := os.MkdirAll(filepath.Dir(bareDir), 0o755); err != nil {
		t.Fatalf("mkdir repos dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		t.Fatalf("mkdir worktrees dir: %v", err)
	}

	runDashboardGit(t, home, "init", "--initial-branch=main", src)
	runDashboardGit(t, src, "config", "user.email", "test@test")
	runDashboardGit(t, src, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("# dash\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runDashboardGit(t, src, "add", ".")
	runDashboardGit(t, src, "commit", "-m", "initial")
	runDashboardGit(t, home, "clone", "--bare", src, bareDir)
	runDashboardGit(t, bareDir, "update-ref", "refs/remotes/origin/main", "main")
	runDashboardGit(t, bareDir, "worktree", "add", wtPath, "main")
	runDashboardGit(t, wtPath, "config", "user.email", "test@test")
	runDashboardGit(t, wtPath, "config", "user.name", "Test")
	return wtPath
}

func writeDashboardSession(t *testing.T, repo, wtDir, sessionID string, status claude.Status, tool string, ts time.Time) {
	t.Helper()

	dir := filepath.Join(claude.StatusDir(), repo, wtDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir status dir: %v", err)
	}
	data, err := json.Marshal(claude.SessionStatus{
		Status:    status,
		SessionID: sessionID,
		Tool:      tool,
		Timestamp: ts,
	})
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, sessionID+".json"), data, 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
}

func resetDiffCache() {
	diffCacheMu.Lock()
	defer diffCacheMu.Unlock()
	diffCache = map[string]cachedDiff{}
}

func TestCollectDataIncludesActiveSessionsUnreadAndDiffStats(t *testing.T) {
	resetDiffCache()
	wtPath := setupDashboardRepo(t)
	if err := os.WriteFile(filepath.Join(wtPath, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatalf("write feature: %v", err)
	}
	runDashboardGit(t, wtPath, "add", "feature.txt")
	runDashboardGit(t, wtPath, "commit", "-m", "feature")

	now := time.Now()
	writeDashboardSession(t, "dashrepo", "main", "busy-session", claude.StatusBusy, "Edit", now)
	writeDashboardSession(t, "dashrepo", "main", "done-session", claude.StatusDone, "", now)

	sessions, sum := collectData()
	if sum.Repos != 1 {
		t.Fatalf("Repos = %d, want 1", sum.Repos)
	}
	if sum.Agents != 2 {
		t.Fatalf("Agents = %d, want 2 active busy/done sessions", sum.Agents)
	}
	if sum.Unread != 1 {
		t.Fatalf("Unread = %d, want 1 done unread session", sum.Unread)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}

	var sawBusy, sawDone bool
	for _, s := range sessions {
		if s.Repo != "dashrepo" || s.Branch != "main" || s.WtDirName != "main" {
			t.Fatalf("unexpected session row: %+v", s)
		}
		if s.DiffStats != "1f +1" {
			t.Fatalf("DiffStats = %q, want 1f +1", s.DiffStats)
		}
		if s.Status == claude.StatusBusy && s.Tool == "Edit" {
			sawBusy = true
		}
		if s.Status == claude.StatusDone && s.Unread {
			sawDone = true
		}
	}
	if !sawBusy || !sawDone {
		t.Fatalf("expected busy and unread done rows, got %+v", sessions)
	}
}

func TestGetDiffStatsCachesByWorktreePath(t *testing.T) {
	resetDiffCache()
	wtPath := setupDashboardRepo(t)
	if err := os.WriteFile(filepath.Join(wtPath, "first.txt"), []byte("first\n"), 0o644); err != nil {
		t.Fatalf("write first: %v", err)
	}
	runDashboardGit(t, wtPath, "add", "first.txt")
	runDashboardGit(t, wtPath, "commit", "-m", "first")

	first := getDiffStats(wtPath, "main")
	if first != "1f +1" {
		t.Fatalf("first diff stats = %q, want 1f +1", first)
	}

	if err := os.WriteFile(filepath.Join(wtPath, "second.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatalf("write second: %v", err)
	}
	runDashboardGit(t, wtPath, "add", "second.txt")
	runDashboardGit(t, wtPath, "commit", "-m", "second")

	second := getDiffStats(wtPath, "main")
	if second != first {
		t.Fatalf("cached diff stats changed before TTL expiry: got %q, want %q", second, first)
	}
}

func TestUpdateFlashesRecordsBusyToDoneAndPrunesMissingSessions(t *testing.T) {
	doneSession := session{
		Repo:      "repo",
		Branch:    "feature",
		SessionID: "s1",
		Status:    claude.StatusDone,
		WtDirName: "feature",
	}
	key := sessionKey(doneSession)
	prev := map[string]claude.Status{key: claude.StatusBusy}
	flashUntil := map[string]time.Time{}

	updateFlashes([]session{doneSession}, prev, flashUntil)
	if prev[key] != claude.StatusDone {
		t.Fatalf("prev[%q] = %s, want DONE", key, prev[key])
	}
	if until, ok := flashUntil[key]; !ok || time.Now().After(until) {
		t.Fatalf("expected fresh flash entry, got %v ok=%v", until, ok)
	}

	updateFlashes(nil, prev, flashUntil)
	if _, ok := prev[key]; ok {
		t.Fatalf("expected missing session to be pruned from prev: %v", prev)
	}
	if _, ok := flashUntil[key]; ok {
		t.Fatalf("expected missing session to be pruned from flashUntil: %v", flashUntil)
	}
}

func TestTermWidthReturnsPositiveFallback(t *testing.T) {
	if got := termWidth(); got <= 0 {
		t.Fatalf("termWidth() = %d, want positive width", got)
	}
}
