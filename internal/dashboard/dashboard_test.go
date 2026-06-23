package dashboard

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iamrajjoshi/willow/internal/agent"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/stack"
)

func TestRenderNoWorktrees(t *testing.T) {
	out := render(nil, summary{Repos: 2, Worktrees: 0, Active: 0, Unread: 0}, 120, 0, nil)
	if !strings.Contains(out, "no worktrees yet") {
		t.Errorf("expected 'no worktrees yet' empty-state copy, got:\n%s", out)
	}
	if !strings.Contains(out, "willow new") {
		t.Errorf("expected 'willow new' hint in empty state, got:\n%s", out)
	}
}

func TestRenderWorktreeRows(t *testing.T) {
	rows := []row{
		{
			Repo:      "myrepo",
			Branch:    "feat--thing",
			Status:    agent.StatusBusy,
			Path:      "/tmp/worktrees/myrepo/feat--thing",
			WtDirName: "feat--thing",
		},
	}
	out := render(rows, summary{Repos: 1, Worktrees: 1, Active: 1, Unread: 0}, 120, 0, nil)

	for _, want := range []string{"WORKTREE", "PATH", "myrepo", "feat--thing", "/tmp/worktrees/myrepo/feat--thing"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
	for _, removed := range []string{"SESSION", "DIFF", "TIMELINE", "j/k: navigate"} {
		if strings.Contains(out, removed) {
			t.Errorf("did not expect %q in passive dashboard output, got:\n%s", removed, out)
		}
	}
}

func TestRenderUnreadMarker(t *testing.T) {
	rows := []row{
		{
			Repo:      "myrepo",
			Branch:    "feat--done",
			Status:    agent.StatusDone,
			Unread:    true,
			Path:      "/tmp/worktrees/myrepo/feat--done",
			WtDirName: "feat--done",
		},
	}
	out := render(rows, summary{Repos: 1, Worktrees: 1, Active: 1, Unread: 1}, 120, 0, nil)

	if !strings.Contains(out, "DONE \u25cf") {
		t.Errorf("expected spaced unread marker in output, got:\n%s", out)
	}
}

func TestRenderStackPrefixAndMergedMarker(t *testing.T) {
	rows := []row{
		{
			Repo:      "repo",
			Branch:    "parent",
			Status:    agent.StatusIdle,
			Path:      "/tmp/worktrees/repo/parent",
			WtDirName: "parent",
		},
		{
			Repo:        "repo",
			Branch:      "child",
			Status:      agent.StatusDone,
			Path:        "/tmp/worktrees/repo/child",
			WtDirName:   "child",
			Merged:      true,
			StackPrefix: "\u2514\u2500 ",
		},
	}
	out := render(rows, summary{Repos: 1, Worktrees: 2, Active: 1, Unread: 0}, 120, 0, nil)

	if !strings.Contains(out, "\u2514\u2500 child") {
		t.Errorf("expected stack prefix before child branch, got:\n%s", out)
	}
	if !strings.Contains(out, "[merged]") {
		t.Errorf("expected merged marker in output, got:\n%s", out)
	}
}

func TestRenderMultiRepoStackNameMatchesPickerStyle(t *testing.T) {
	rows := []row{
		{
			Repo:      "repo-a",
			Branch:    "main",
			Status:    agent.StatusOffline,
			Path:      "/tmp/worktrees/repo-a/main",
			WtDirName: "main",
		},
		{
			Repo:        "repo-b",
			Branch:      "stack-child",
			Status:      agent.StatusWait,
			Path:        "/tmp/worktrees/repo-b/stack-child",
			WtDirName:   "stack-child",
			StackPrefix: "\u2514\u2500 ",
		},
	}
	out := render(rows, summary{Repos: 2, Worktrees: 2, Active: 1, Unread: 0}, 120, 0, nil)

	if !strings.Contains(out, "\u2514\u2500 repo-b/stack-child") {
		t.Errorf("expected multi-repo stack label to match picker style, got:\n%s", out)
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

func addDashboardWorktree(t *testing.T, bareDir, branch, base string) string {
	t.Helper()

	wtPath := filepath.Join(config.WorktreesDir(), "dashrepo", branch)
	runDashboardGit(t, bareDir, "branch", branch, base)
	runDashboardGit(t, bareDir, "worktree", "add", wtPath, branch)
	return wtPath
}

func writeDashboardSession(t *testing.T, repo, wtDir, sessionID string, status agent.Status, tool string, ts time.Time) {
	t.Helper()

	dir := filepath.Join(agent.StatusDir(), repo, wtDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir status dir: %v", err)
	}
	data, err := json.Marshal(agent.SessionStatus{
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

func TestCollectDataIncludesAllWorktreesUnreadAndStackPrefixes(t *testing.T) {
	setupDashboardRepo(t)
	bareDir := filepath.Join(config.ReposDir(), "dashrepo.git")
	addDashboardWorktree(t, bareDir, "feature-a", "main")
	addDashboardWorktree(t, bareDir, "feature-b", "feature-a")

	st := &stack.Stack{Parents: map[string]string{
		"feature-a": "main",
		"feature-b": "feature-a",
	}}
	if err := st.Save(bareDir); err != nil {
		t.Fatalf("save stack: %v", err)
	}

	now := time.Now()
	writeDashboardSession(t, "dashrepo", "feature-a", "done-session", agent.StatusDone, "", now)
	writeDashboardSession(t, "dashrepo", "feature-b", "busy-session", agent.StatusBusy, "Edit", now)
	writeDashboardSession(t, "dashrepo", "feature-b", "done-session", agent.StatusDone, "", now)

	rows, sum := collectData(context.Background())
	if sum.Repos != 1 {
		t.Fatalf("Repos = %d, want 1", sum.Repos)
	}
	if sum.Worktrees != 3 {
		t.Fatalf("Worktrees = %d, want 3", sum.Worktrees)
	}
	if sum.Active != 2 {
		t.Fatalf("Active = %d, want 2", sum.Active)
	}
	if sum.Unread != 1 {
		t.Fatalf("Unread = %d, want 1", sum.Unread)
	}
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3: %+v", len(rows), rows)
	}

	var sawMain, sawFeatureA, sawFeatureB bool
	for _, r := range rows {
		switch r.Branch {
		case "main":
			sawMain = true
			if r.Status != agent.StatusOffline {
				t.Fatalf("main status = %s, want offline", r.Status)
			}
		case "feature-a":
			sawFeatureA = true
			if !r.Unread {
				t.Fatalf("feature-a should carry unread marker: %+v", r)
			}
		case "feature-b":
			sawFeatureB = true
			if r.Status != agent.StatusBusy {
				t.Fatalf("feature-b status = %s, want BUSY", r.Status)
			}
			if r.Unread {
				t.Fatalf("feature-b should not show unread marker while BUSY: %+v", r)
			}
			if r.StackPrefix == "" {
				t.Fatalf("feature-b should have stack prefix: %+v", r)
			}
		}
	}
	if !sawMain || !sawFeatureA || !sawFeatureB {
		t.Fatalf("missing expected rows: %+v", rows)
	}
}

func TestUpdateFlashesRecordsBusyToDoneAndPrunesMissingRows(t *testing.T) {
	doneRow := row{
		Repo:      "repo",
		Branch:    "feature",
		Status:    agent.StatusDone,
		WtDirName: "feature",
	}
	key := rowKey(doneRow)
	prev := map[string]agent.Status{key: agent.StatusBusy}
	flashUntil := map[string]time.Time{}

	updateFlashes([]row{doneRow}, prev, flashUntil)
	if prev[key] != agent.StatusDone {
		t.Fatalf("prev[%q] = %s, want DONE", key, prev[key])
	}
	if until, ok := flashUntil[key]; !ok || time.Now().After(until) {
		t.Fatalf("expected fresh flash entry, got %v ok=%v", until, ok)
	}

	updateFlashes(nil, prev, flashUntil)
	if _, ok := prev[key]; ok {
		t.Fatalf("expected missing row to be pruned from prev: %v", prev)
	}
	if _, ok := flashUntil[key]; ok {
		t.Fatalf("expected missing row to be pruned from flashUntil: %v", flashUntil)
	}
}

func TestTermWidthReturnsPositiveFallback(t *testing.T) {
	if got := termWidth(); got <= 0 {
		t.Fatalf("termWidth() = %d, want positive width", got)
	}
}
