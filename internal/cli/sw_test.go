package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/worktree"
)

func TestExtractPathFromLine(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"🤖 BUSY  main  /home/user/.willow/worktrees/repo/main", "/home/user/.willow/worktrees/repo/main"},
		{"✅ DONE  feature/auth  /wt/repo/feature-auth", "/wt/repo/feature-auth"},
		{"🟡 IDLE  repo/branch  /some/path", "/some/path"},
		{"", ""},
		{"single-field", "single-field"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := extractPathFromLine(tt.line)
			if got != tt.want {
				t.Errorf("extractPathFromLine(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func writeSessionStatus(t *testing.T, home, repo, wt string, status claude.Status, ts time.Time) {
	t.Helper()
	dir := filepath.Join(home, ".willow", "status", repo, wt)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir status dir: %v", err)
	}

	data, err := json.Marshal(claude.SessionStatus{
		Status:    status,
		SessionID: wt + "-session",
		Timestamp: ts,
		Worktree:  wt,
	})
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, wt+"-session.json"), data, 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
}
func TestBuildWorktreeLines(t *testing.T) {
	// Use a temp HOME so claude.ReadStatus returns offline for all
	home := t.TempDir()
	t.Setenv("HOME", home)

	wts := []worktree.Worktree{
		{Branch: "main", Path: "/wt/repo/main"},
		{Branch: "feature/auth", Path: "/wt/repo/feature-auth"},
	}

	lines := buildWorktreeLines(wts, "repo")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	for _, line := range lines {
		// Each line should contain a path
		if !strings.Contains(line, "/wt/repo/") {
			t.Errorf("line should contain path, got %q", line)
		}
	}
}

func TestBuildWorktreeLines_UrgencyOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	now := time.Now().UTC()
	writeSessionStatus(t, home, "repo", "busy", claude.StatusBusy, now)
	writeSessionStatus(t, home, "repo", "done-unread", claude.StatusDone, now)
	writeSessionStatus(t, home, "repo", "wait", claude.StatusWait, now)
	writeSessionStatus(t, home, "repo", "done-read", claude.StatusDone, now.Add(-2*time.Minute))
	readPath := filepath.Join(home, ".willow", "status", "repo", "done-read", ".lastread")
	if err := os.WriteFile(readPath, []byte(now.UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
		t.Fatalf("write lastread: %v", err)
	}

	wts := []worktree.Worktree{
		{Branch: "busy", Path: "/wt/repo/busy"},
		{Branch: "done-unread", Path: "/wt/repo/done-unread"},
		{Branch: "wait", Path: "/wt/repo/wait"},
		{Branch: "done-read", Path: "/wt/repo/done-read"},
	}

	lines := buildWorktreeLines(wts, "repo")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(lines))
	}

	got := []string{
		extractPathFromLine(lines[0]),
		extractPathFromLine(lines[1]),
		extractPathFromLine(lines[2]),
		extractPathFromLine(lines[3]),
	}
	want := []string{
		"/wt/repo/wait",
		"/wt/repo/done-unread",
		"/wt/repo/busy",
		"/wt/repo/done-read",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("path[%d] = %q, want %q (full order %v)", i, got[i], want[i], got)
		}
	}
	if !strings.Contains(lines[1], "DONE●") {
		t.Fatalf("expected unread DONE marker in line %q", lines[1])
	}
}

func TestBuildCrossRepoWorktreeLines(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rwts := []repoWorktree{
		{
			Repo:     repoInfo{Name: "alpha", BareDir: "/repos/alpha.git"},
			Worktree: worktree.Worktree{Branch: "main", Path: "/wt/alpha/main"},
		},
		{
			Repo:     repoInfo{Name: "beta", BareDir: "/repos/beta.git"},
			Worktree: worktree.Worktree{Branch: "feature", Path: "/wt/beta/feature"},
		},
	}

	lines := buildCrossRepoWorktreeLines(rwts)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Lines should contain repo/branch format
	foundAlpha := false
	foundBeta := false
	for _, line := range lines {
		if strings.Contains(line, "alpha/main") {
			foundAlpha = true
		}
		if strings.Contains(line, "beta/feature") {
			foundBeta = true
		}
		// Path should be the last field
		path := extractPathFromLine(line)
		if !strings.HasPrefix(path, "/wt/") {
			t.Errorf("expected path to start with /wt/, got %q", path)
		}
	}
	if !foundAlpha {
		t.Error("expected alpha/main in lines")
	}
	if !foundBeta {
		t.Error("expected beta/feature in lines")
	}
}

func TestBuildCrossRepoWorktreeLines_UrgencyOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	now := time.Now().UTC()
	writeSessionStatus(t, home, "alpha", "busy", claude.StatusBusy, now)
	writeSessionStatus(t, home, "beta", "done-unread", claude.StatusDone, now)
	writeSessionStatus(t, home, "gamma", "wait", claude.StatusWait, now)

	rwts := []repoWorktree{
		{
			Repo:     repoInfo{Name: "alpha", BareDir: "/repos/alpha.git"},
			Worktree: worktree.Worktree{Branch: "busy", Path: "/wt/alpha/busy"},
		},
		{
			Repo:     repoInfo{Name: "beta", BareDir: "/repos/beta.git"},
			Worktree: worktree.Worktree{Branch: "done-unread", Path: "/wt/beta/done-unread"},
		},
		{
			Repo:     repoInfo{Name: "gamma", BareDir: "/repos/gamma.git"},
			Worktree: worktree.Worktree{Branch: "wait", Path: "/wt/gamma/wait"},
		},
	}

	lines := buildCrossRepoWorktreeLines(rwts)
	got := []string{
		extractPathFromLine(lines[0]),
		extractPathFromLine(lines[1]),
		extractPathFromLine(lines[2]),
	}
	want := []string{
		"/wt/gamma/wait",
		"/wt/beta/done-unread",
		"/wt/alpha/busy",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("path[%d] = %q, want %q (full order %v)", i, got[i], want[i], got)
		}
	}
}

func TestSwCommandPickerSingleRepoPrintsSelectedPath(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	if err := runApp("clone", origin, "swpicker"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	t.Setenv("FZF_DEFAULT_OPTS", "--filter=master")

	out, err := captureStdout(t, func() error {
		return runApp("sw", "--repo", "swpicker")
	})
	if err != nil {
		t.Fatalf("sw picker failed: %v", err)
	}
	wtDir := firstWorktreeDir(t, filepath.Join(home, ".willow", "worktrees", "swpicker"))
	if !strings.Contains(out, filepath.Join(".willow", "worktrees", "swpicker", wtDir)) {
		t.Fatalf("sw output = %q, want selected worktree path", out)
	}
}

func TestSwCommandPickerMultiRepoPrintsSelectedPath(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	if err := runApp("clone", origin, "swone"); err != nil {
		t.Fatalf("clone swone failed: %v", err)
	}
	if err := runApp("clone", origin, "swtwo"); err != nil {
		t.Fatalf("clone swtwo failed: %v", err)
	}
	if err := os.Chdir(home); err != nil {
		t.Fatalf("chdir home: %v", err)
	}
	t.Setenv("FZF_DEFAULT_OPTS", "--filter=swtwo/")

	out, err := captureStdout(t, func() error {
		return runApp("sw")
	})
	if err != nil {
		t.Fatalf("sw multi picker failed: %v", err)
	}
	if !strings.Contains(out, filepath.Join(".willow", "worktrees", "swtwo")) {
		t.Fatalf("sw multi output = %q, want swtwo path", out)
	}
}

func TestSwShellCompletionWithRepoFlagListsWorktrees(t *testing.T) {
	origin := setupTestEnv(t)
	if err := runApp("clone", origin, "swcomplete"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runApp("sw", "--repo", "swcomplete", "--generate-shell-completion")
	})
	if err != nil {
		t.Fatalf("sw shell completion failed: %v", err)
	}
	if !strings.Contains(out, "master") && !strings.Contains(out, "main") {
		t.Fatalf("completion output = %q, want worktree match name", out)
	}
}

func TestSwShellCompletionCrossRepoFallbackListsAllWorktrees(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	if err := runApp("clone", origin, "swcompone"); err != nil {
		t.Fatalf("clone swcompone failed: %v", err)
	}
	if err := runApp("clone", origin, "swcomptwo"); err != nil {
		t.Fatalf("clone swcomptwo failed: %v", err)
	}
	if err := os.Chdir(home); err != nil {
		t.Fatalf("chdir home: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runApp("sw", "--generate-shell-completion")
	})
	if err != nil {
		t.Fatalf("sw cross-repo shell completion failed: %v", err)
	}
	if !strings.Contains(out, "master") && !strings.Contains(out, "main") {
		t.Fatalf("cross-repo completion output = %q, want worktree names", out)
	}
}

func TestFzfPickWorktreeFilterModeReturnsSelectedPathAndHandlesNoMatch(t *testing.T) {
	wts := []worktree.Worktree{
		{Branch: "main", Path: "/wt/repo/main"},
		{Branch: "feature", Path: "/wt/repo/feature"},
	}
	t.Setenv("FZF_DEFAULT_OPTS", "--filter=feature")

	got, err := fzfPickWorktree(wts, "repo")
	if err != nil {
		t.Fatalf("fzfPickWorktree: %v", err)
	}
	if got != "/wt/repo/feature" {
		t.Fatalf("fzfPickWorktree() = %q, want feature path", got)
	}

	t.Setenv("FZF_DEFAULT_OPTS", "--filter=missing")
	got, err = fzfPickWorktree(wts, "repo")
	if err != nil || got != "" {
		t.Fatalf("fzfPickWorktree no match = %q, %v; want empty, nil", got, err)
	}
}

func TestFzfPickWorktreesFilterModeReturnsSelectedPaths(t *testing.T) {
	wts := []worktree.Worktree{
		{Branch: "main", Path: "/wt/repo/main"},
		{Branch: "feature", Path: "/wt/repo/feature"},
	}
	t.Setenv("FZF_DEFAULT_OPTS", "--filter=repo/")

	got, err := fzfPickWorktrees(wts, "repo")
	if err != nil {
		t.Fatalf("fzfPickWorktrees: %v", err)
	}
	want := []string{"/wt/repo/feature", "/wt/repo/main"}
	for _, path := range want {
		if !containsString(got, path) {
			t.Fatalf("fzfPickWorktrees = %v, want %s", got, path)
		}
	}
}
