package tmux

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/stack"
)

func TestStripAnsi(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text unchanged", "hello world", "hello world"},
		{"single ANSI code stripped", "\033[0;32mgreen\033[0m", "green"},
		{"multiple escapes", "\033[0;31mred\033[0m and \033[0;34mblue\033[0m", "red and blue"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripAnsi(tt.input)
			if got != tt.want {
				t.Errorf("stripAnsi(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExpandHome(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"tilde prefix expands", "~/foo", "/fakehome/foo"},
		{"absolute path unchanged", "/usr/local/bin", "/usr/local/bin"},
		{"relative path unchanged", "relative/path", "relative/path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", "/fakehome")
			got := expandHome(tt.path)
			if got != tt.want {
				t.Errorf("expandHome(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestShortenPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"home prefix becomes tilde", "/fakehome/code/project", "~/code/project"},
		{"non-home path unchanged", "/other/path", "/other/path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", "/fakehome")
			got := shortenPath(tt.path)
			if got != tt.want {
				t.Errorf("shortenPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		n     int
		want  string
	}{
		{"shorter than limit", "hi", 10, "hi"},
		{"longer than limit", "abcdefghij", 5, "abcde"},
		{"exact length", "abc", 3, "abc"},
		{"empty string", "", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.n)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
			}
		})
	}
}

func TestStatusColor(t *testing.T) {
	tests := []struct {
		name   string
		status claude.Status
		want   string
	}{
		{"BUSY is green", claude.StatusBusy, colorGreen},
		{"WAIT is red", claude.StatusWait, colorRed},
		{"DONE is blue", claude.StatusDone, colorBlue},
		{"IDLE is yellow", claude.StatusIdle, colorYellow},
		{"OFFLINE is dim", claude.StatusOffline, colorDim},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusColor(tt.status)
			if got != tt.want {
				t.Errorf("statusColor(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestDisplayName(t *testing.T) {
	item := PickerItem{RepoName: "myrepo", Branch: "feat-auth"}

	tests := []struct {
		name      string
		multiRepo bool
		want      string
	}{
		{"single repo shows branch only", false, "feat-auth"},
		{"multi repo shows repo/branch", true, "myrepo/feat-auth"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := displayName(item, tt.multiRepo)
			if got != tt.want {
				t.Errorf("displayName(item, %v) = %q, want %q", tt.multiRepo, got, tt.want)
			}
		})
	}
}

func TestDisplayName_Detached(t *testing.T) {
	item := PickerItem{
		RepoName:  "myrepo",
		Branch:    "(detached)",
		Head:      "abcdef123456",
		Detached:  true,
		WtDirName: "scratch-repro",
	}

	if got := displayName(item, false); got != "scratch-repro [detached abcdef1]" {
		t.Fatalf("displayName(single repo) = %q", got)
	}
	if got := displayName(item, true); got != "myrepo/scratch-repro [detached abcdef1]" {
		t.Fatalf("displayName(multi repo) = %q", got)
	}
}

func TestHasMultipleRepos(t *testing.T) {
	tests := []struct {
		name  string
		items []PickerItem
		want  bool
	}{
		{"empty slice", nil, false},
		{"single item", []PickerItem{{RepoName: "a"}}, false},
		{"same repo items", []PickerItem{{RepoName: "a"}, {RepoName: "a"}}, false},
		{"mixed repos", []PickerItem{{RepoName: "a"}, {RepoName: "b"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasMultipleRepos(tt.items)
			if got != tt.want {
				t.Errorf("hasMultipleRepos() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterActiveSessions(t *testing.T) {
	now := time.Now()
	stale := time.Now().Add(-5 * time.Minute)

	tests := []struct {
		name     string
		sessions []*claude.SessionStatus
		wantLen  int
	}{
		{
			"mix of active and idle",
			[]*claude.SessionStatus{
				{Status: claude.StatusBusy, Timestamp: now},
				{Status: claude.StatusIdle, Timestamp: now},
				{Status: claude.StatusWait, Timestamp: now},
			},
			2,
		},
		{
			"all idle returns empty",
			[]*claude.SessionStatus{
				{Status: claude.StatusIdle, Timestamp: now},
				{Status: claude.StatusIdle, Timestamp: now},
			},
			0,
		},
		{
			"all active",
			[]*claude.SessionStatus{
				{Status: claude.StatusBusy, Timestamp: now},
				{Status: claude.StatusDone, Timestamp: now},
				{Status: claude.StatusWait, Timestamp: now},
			},
			3,
		},
		{
			"stale BUSY/WAIT become idle, stale DONE stays",
			[]*claude.SessionStatus{
				{Status: claude.StatusBusy, Timestamp: stale},
				{Status: claude.StatusDone, Timestamp: stale},
				{Status: claude.StatusWait, Timestamp: stale},
			},
			1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterActiveSessions(tt.sessions)
			if len(got) != tt.wantLen {
				t.Errorf("filterActiveSessions() returned %d sessions, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestExtractPathFromLine(t *testing.T) {
	t.Setenv("HOME", "/fakehome")

	tests := []struct {
		name string
		line string
		want string
	}{
		{
			"formatted picker line",
			"\033[0;32m\U0001F916 BUSY   \033[0m | feat-auth | \033[2m~/code/project\033[0m",
			"/fakehome/code/project",
		},
		{
			"sub-row line",
			"  \033[0;32m\U0001F916 BUSY \033[0m | \033[2m\u2514 abcd1234 5m ago\033[2m\033[0m | \033[2m~/code/project\033[0m",
			"/fakehome/code/project",
		},
		{
			"no pipes returns original expanded",
			"just-a-string",
			"just-a-string",
		},
		{
			"empty string",
			"",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPathFromLine(tt.line)
			if got != tt.want {
				t.Errorf("ExtractPathFromLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatPickerLinesMultiRepoMergedUnreadAndSubSessions(t *testing.T) {
	t.Setenv("HOME", "/fakehome")
	now := time.Now()

	items := []PickerItem{
		{
			RepoName:  "repo-a",
			Branch:    "done-branch",
			WtPath:    "/fakehome/worktrees/repo-a/done-branch",
			Status:    claude.StatusDone,
			Unread:    true,
			Merged:    true,
			WtDirName: "done-branch",
		},
		{
			RepoName:    "repo-b",
			Branch:      "stack-child",
			WtPath:      "/fakehome/worktrees/repo-b/stack-child",
			Status:      claude.StatusBusy,
			StackPrefix: "└─ ",
			Sessions: []*claude.SessionStatus{
				{SessionID: "busy-session-123", Status: claude.StatusBusy, Tool: "Edit", Timestamp: now},
				{SessionID: "done-session-456", Status: claude.StatusDone, Timestamp: now},
			},
			WtDirName: "stack-child",
		},
	}

	lines := FormatPickerLines(items)
	if len(lines) != 4 {
		t.Fatalf("FormatPickerLines() returned %d lines, want 4: %#v", len(lines), lines)
	}

	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"repo-a/done-branch",
		"[merged]",
		"●",
		"~/worktrees/repo-a/done-branch",
		"└─ repo-b/stack-child",
		"busy-se",
		"(Edit)",
		"done-se",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("formatted picker lines missing %q:\n%s", want, joined)
		}
	}
}

func TestBuildPickerItemsUsesRepoWorktreesAndStatuses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bareDir := pickerTestBareRepo(t, home, "repo")
	wtPath := filepath.Join(home, ".willow", "worktrees", "repo", "main")
	if _, err := (&git.Git{Dir: bareDir}).Run("worktree", "add", wtPath, "main"); err != nil {
		t.Fatalf("git worktree add: %v", err)
	}
	writePickerSessionStatus(t, "repo", "main", claude.StatusDone)

	items, err := BuildPickerItemsWithOptions(contextBackground(), "repo", PickerBuildOptions{RefreshGitHubMerged: false})
	if err != nil {
		t.Fatalf("BuildPickerItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("BuildPickerItems returned %d items, want 1: %+v", len(items), items)
	}
	item := items[0]
	resolvedWtPath, err := filepath.EvalSymlinks(wtPath)
	if err != nil {
		t.Fatalf("resolve worktree path: %v", err)
	}
	if item.RepoName != "repo" || item.Branch != "main" || item.WtPath != resolvedWtPath {
		t.Fatalf("picker item = %+v, want repo/main at %s", item, wtPath)
	}
	if item.Status != claude.StatusDone || !item.Unread {
		t.Fatalf("picker item status/unread = %s/%v, want DONE/unread", item.Status, item.Unread)
	}
}

func TestBuildPickerItemsMissingRepoReturnsEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	items, err := BuildPickerItems(contextBackground(), "missing")
	if err != nil {
		t.Fatalf("BuildPickerItems missing repo error = %v, want nil", err)
	}
	if len(items) != 0 {
		t.Fatalf("BuildPickerItems missing repo = %+v, want empty", items)
	}
}

func TestDefaultPickerStackLoader(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bareDir := filepath.Join(home, ".willow", "repos", "repo.git")
	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatalf("mkdir bare dir: %v", err)
	}
	st := pickerStack("child", "main")
	if err := st.Save(bareDir); err != nil {
		t.Fatalf("save stack: %v", err)
	}
	got := defaultPickerStackLoader("repo")
	if got == nil || got.Parent("child") != "main" {
		t.Fatalf("defaultPickerStackLoader() = %+v, want child parent main", got)
	}
	if got := defaultPickerStackLoader("missing"); got != nil {
		t.Fatalf("defaultPickerStackLoader missing = %+v, want nil", got)
	}
}

func pickerStack(pairs ...string) *stack.Stack {
	st := &stack.Stack{Parents: make(map[string]string)}
	for i := 0; i+1 < len(pairs); i += 2 {
		st.Parents[pairs[i]] = pairs[i+1]
	}
	return st
}

func pickerTestBareRepo(t *testing.T, home, repo string) string {
	t.Helper()
	bareDir := filepath.Join(home, ".willow", "repos", repo+".git")
	if err := os.MkdirAll(filepath.Dir(bareDir), 0o755); err != nil {
		t.Fatalf("mkdir repos dir: %v", err)
	}
	if _, err := (&git.Git{}).Run("init", "--bare", bareDir); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}
	seed := filepath.Join(home, "seed")
	if _, err := (&git.Git{}).Run("clone", bareDir, seed); err != nil {
		t.Fatalf("git clone seed: %v", err)
	}
	seedGit := &git.Git{Dir: seed}
	if _, err := seedGit.Run("config", "user.email", "test@test.com"); err != nil {
		t.Fatalf("git config email: %v", err)
	}
	if _, err := seedGit.Run("config", "user.name", "Test"); err != nil {
		t.Fatalf("git config name: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("# repo\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if _, err := seedGit.Run("add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := seedGit.Run("commit", "-m", "initial"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	if _, err := seedGit.Run("branch", "-M", "main"); err != nil {
		t.Fatalf("git branch main: %v", err)
	}
	if _, err := seedGit.Run("push", "origin", "main"); err != nil {
		t.Fatalf("git push main: %v", err)
	}
	return bareDir
}

func writePickerSessionStatus(t *testing.T, repo, wt string, status claude.Status) {
	t.Helper()
	dir := filepath.Join(claude.StatusDir(), repo, wt)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir status dir: %v", err)
	}
	data, err := json.Marshal(claude.SessionStatus{
		Status:    status,
		SessionID: "session-1",
		Timestamp: time.Now(),
		Worktree:  wt,
	})
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session-1.json"), data, 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}
}

func contextBackground() context.Context {
	return context.Background()
}

func pickerLoader(stacks map[string]*stack.Stack) pickerStackLoader {
	return func(repoName string) *stack.Stack {
		return stacks[repoName]
	}
}

func pickerBranches(items []PickerItem) []string {
	branches := make([]string, len(items))
	for i, item := range items {
		branches[i] = item.Branch
	}
	return branches
}

func TestSortPickerItems_UrgencyOrder(t *testing.T) {
	items := []PickerItem{
		{RepoName: "repo", Branch: "busy", Status: claude.StatusBusy},
		{RepoName: "repo", Branch: "done-unread", Status: claude.StatusDone, Unread: true},
		{RepoName: "repo", Branch: "wait", Status: claude.StatusWait},
		{RepoName: "repo", Branch: "done-read", Status: claude.StatusDone},
		{RepoName: "repo", Branch: "idle", Status: claude.StatusIdle},
		{RepoName: "repo", Branch: "offline", Status: claude.StatusOffline},
	}

	got := pickerBranches(sortPickerItems(items, []string{"repo"}, pickerLoader(nil)))
	want := []string{"wait", "done-unread", "busy", "done-read", "idle", "offline"}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("branch[%d] = %q, want %q (full order %v)", i, got[i], want[i], got)
		}
	}
}

func TestSortPickerItems_MergedLast(t *testing.T) {
	items := []PickerItem{
		{RepoName: "repo", Branch: "merged-wait", Status: claude.StatusWait, Merged: true},
		{RepoName: "repo", Branch: "busy", Status: claude.StatusBusy},
	}

	got := pickerBranches(sortPickerItems(items, []string{"repo"}, pickerLoader(nil)))
	want := []string{"busy", "merged-wait"}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("branch[%d] = %q, want %q (full order %v)", i, got[i], want[i], got)
		}
	}
}

func TestSortPickerItems_StackStaysContiguousAndRanksByUrgency(t *testing.T) {
	items := []PickerItem{
		{RepoName: "repo", Branch: "busy", Status: claude.StatusBusy},
		{RepoName: "repo", Branch: "stack-a", Status: claude.StatusIdle},
		{RepoName: "repo", Branch: "stack-b", Status: claude.StatusWait},
		{RepoName: "repo", Branch: "done-read", Status: claude.StatusDone},
	}

	got := sortPickerItems(items, []string{"repo"}, pickerLoader(map[string]*stack.Stack{
		"repo": pickerStack("stack-a", "main", "stack-b", "stack-a"),
	}))

	want := []string{"stack-a", "stack-b", "busy", "done-read"}
	branches := pickerBranches(got)
	for i := range want {
		if branches[i] != want[i] {
			t.Fatalf("branch[%d] = %q, want %q (full order %v)", i, branches[i], want[i], branches)
		}
	}
	if got[0].StackPrefix != "" {
		t.Fatalf("root stack prefix = %q, want empty", got[0].StackPrefix)
	}
	if got[1].StackPrefix == "" {
		t.Fatal("child stack prefix should be preserved")
	}
}

func TestSortPickerItems_EqualPriorityKeepsStableOrder(t *testing.T) {
	items := []PickerItem{
		{RepoName: "repo", Branch: "busy-a", Status: claude.StatusBusy},
		{RepoName: "repo", Branch: "busy-b", Status: claude.StatusBusy},
		{RepoName: "repo", Branch: "busy-c", Status: claude.StatusBusy},
	}

	got := pickerBranches(sortPickerItems(items, []string{"repo"}, pickerLoader(nil)))
	want := []string{"busy-a", "busy-b", "busy-c"}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("branch[%d] = %q, want %q (full order %v)", i, got[i], want[i], got)
		}
	}
}
