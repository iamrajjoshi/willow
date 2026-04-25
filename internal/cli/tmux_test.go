package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/tmux"
	"github.com/iamrajjoshi/willow/internal/worktree"
)

func item(repo, branch string) tmux.PickerItem {
	return tmux.PickerItem{
		RepoName:  repo,
		Branch:    branch,
		WtDirName: branch, // simplification: dir == branch for tests
		WtPath:    "/fake/" + repo + "/" + branch,
	}
}

func branches(items []tmux.PickerItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Branch
	}
	return out
}

func mockLoader(stacks map[string]*stack.Stack) stackLoaderFunc {
	return func(repoName string) *stack.Stack {
		return stacks[repoName]
	}
}

func nilLoader(_ string) *stack.Stack {
	return nil
}

// makeStack builds a Stack from parent→child pairs.
func makeStack(pairs ...string) *stack.Stack {
	s := &stack.Stack{Parents: make(map[string]string)}
	for i := 0; i+1 < len(pairs); i += 2 {
		child, parent := pairs[i], pairs[i+1]
		s.Parents[child] = parent
	}
	return s
}

func TestMoveToFrontWithStack_NonStacked(t *testing.T) {
	items := []tmux.PickerItem{
		item("repo", "a"),
		item("repo", "b"),
		item("repo", "c"),
	}

	// Session "repo/b" is not in any stack
	result := moveToFrontWithStack(items, "repo/b", nilLoader)
	got := branches(result)
	want := []string{"b", "a", "c"}
	assertBranches(t, got, want)
}

func TestMoveToFrontWithStack_NoMatch(t *testing.T) {
	items := []tmux.PickerItem{
		item("repo", "a"),
		item("repo", "b"),
	}

	result := moveToFrontWithStack(items, "repo/z", nilLoader)
	got := branches(result)
	want := []string{"a", "b"}
	assertBranches(t, got, want)
}

func TestMoveToFrontWithStack_RootIsCurrent(t *testing.T) {
	// Stack: main → feat-a → feat-b
	st := makeStack("feat-a", "main", "feat-b", "feat-a")
	loader := mockLoader(map[string]*stack.Stack{"repo": st})

	items := []tmux.PickerItem{
		item("repo", "x"),
		item("repo", "feat-a"),
		item("repo", "y"),
		item("repo", "feat-b"),
		item("repo", "z"),
	}

	// Current session is the stack root (feat-a)
	result := moveToFrontWithStack(items, "repo/feat-a", loader)
	got := branches(result)
	want := []string{"feat-a", "feat-b", "x", "y", "z"}
	assertBranches(t, got, want)
}

func TestMoveToFrontWithStack_ChildIsCurrent(t *testing.T) {
	// Stack: main → feat-a → feat-b → feat-c
	st := makeStack("feat-a", "main", "feat-b", "feat-a", "feat-c", "feat-b")
	loader := mockLoader(map[string]*stack.Stack{"repo": st})

	items := []tmux.PickerItem{
		item("repo", "other"),
		item("repo", "feat-a"),
		item("repo", "feat-b"),
		item("repo", "feat-c"),
	}

	// Current session is a child (feat-c) — should walk up to feat-a and move whole tree
	result := moveToFrontWithStack(items, "repo/feat-c", loader)
	got := branches(result)
	want := []string{"feat-a", "feat-b", "feat-c", "other"}
	assertBranches(t, got, want)
}

func TestMoveToFrontWithStack_MultipleTrees(t *testing.T) {
	// Two trees:
	// Tree 1: main → a → b
	// Tree 2: main → x → y
	st := makeStack("a", "main", "b", "a", "x", "main", "y", "x")
	loader := mockLoader(map[string]*stack.Stack{"repo": st})

	items := []tmux.PickerItem{
		item("repo", "other"),
		item("repo", "x"),
		item("repo", "a"),
		item("repo", "y"),
		item("repo", "b"),
	}

	// Match tree 1 via child "b" — only tree 1 moves, tree 2 stays
	result := moveToFrontWithStack(items, "repo/b", loader)
	got := branches(result)
	want := []string{"a", "b", "other", "x", "y"}
	assertBranches(t, got, want)
}

func TestMoveToFrontWithStack_LoaderReturnsNil(t *testing.T) {
	items := []tmux.PickerItem{
		item("repo", "a"),
		item("repo", "b"),
		item("repo", "c"),
	}

	// Loader returns nil — falls back to single-item move
	result := moveToFrontWithStack(items, "repo/b", nilLoader)
	got := branches(result)
	want := []string{"b", "a", "c"}
	assertBranches(t, got, want)
}

func TestMoveToFrontWithStack_CrossRepoSameBranchName(t *testing.T) {
	// Both repos have "feat-a" but only repo1 has a stack
	st := makeStack("feat-a", "main", "feat-b", "feat-a")
	loader := mockLoader(map[string]*stack.Stack{"repo1": st})

	items := []tmux.PickerItem{
		item("repo2", "feat-a"), // same branch name, different repo
		item("repo1", "feat-a"),
		item("repo1", "feat-b"),
		item("repo1", "other"),
	}

	// Match repo1/feat-a — should move repo1's tree, not repo2's item
	result := moveToFrontWithStack(items, "repo1/feat-a", loader)
	_ = branches(result) // used for debugging
	// repo1 tree first, then repo2's feat-a, then repo1 other
	if len(result) != 4 {
		t.Fatalf("expected 4 items, got %d", len(result))
	}
	// Tree items (repo1) come first, preserving order
	if result[0].RepoName != "repo1" || result[0].Branch != "feat-a" {
		t.Errorf("result[0] = %s/%s, want repo1/feat-a", result[0].RepoName, result[0].Branch)
	}
	if result[1].RepoName != "repo1" || result[1].Branch != "feat-b" {
		t.Errorf("result[1] = %s/%s, want repo1/feat-b", result[1].RepoName, result[1].Branch)
	}
	// repo2's feat-a stays in the rest
	if result[2].RepoName != "repo2" || result[2].Branch != "feat-a" {
		t.Errorf("result[2] = %s/%s, want repo2/feat-a", result[2].RepoName, result[2].Branch)
	}
	if result[3].RepoName != "repo1" || result[3].Branch != "other" {
		t.Errorf("result[3] = %s/%s, want repo1/other", result[3].RepoName, result[3].Branch)
	}
}

func TestMoveToFrontWithStack_BranchNotInStack(t *testing.T) {
	// Stack exists but matched branch is not in it
	st := makeStack("feat-a", "main")
	loader := mockLoader(map[string]*stack.Stack{"repo": st})

	items := []tmux.PickerItem{
		item("repo", "feat-a"),
		item("repo", "unrelated"),
		item("repo", "other"),
	}

	// "unrelated" is not tracked in the stack — single-item fallback
	result := moveToFrontWithStack(items, "repo/unrelated", loader)
	got := branches(result)
	want := []string{"unrelated", "feat-a", "other"}
	assertBranches(t, got, want)
}

func TestMoveToFrontWithStack_DetachedOnlyMovesSelectedWorktree(t *testing.T) {
	st := makeStack("feat-b", "feat-a")
	loader := mockLoader(map[string]*stack.Stack{"repo": st})

	items := []tmux.PickerItem{
		item("repo", "feat-a"),
		{
			RepoName:  "repo",
			Branch:    worktree.DetachedBranch,
			Detached:  true,
			WtDirName: "scratch-repro",
			WtPath:    "/fake/repo/scratch-repro",
		},
		item("repo", "feat-b"),
	}

	result := moveToFrontWithStack(items, "repo/scratch-repro", loader)
	if result[0].WtDirName != "scratch-repro" || !result[0].Detached {
		t.Fatalf("first item = %#v, want detached scratch-repro", result[0])
	}
	assertBranches(t, branches(result[1:]), []string{"feat-a", "feat-b"})
}

func TestTmuxPickerHeaderFitsEightyColumns(t *testing.T) {
	if got := len(tmuxPickerHeader); got > 80 {
		t.Fatalf("tmux picker header length = %d, want <= 80", got)
	}
	if strings.Contains(tmuxPickerHeader, "upgrade") {
		t.Fatal("tmux picker header should not mention the removed upgrade alias")
	}
}

func TestTmuxPickDetachedArgs(t *testing.T) {
	got := tmuxPickDetachedArgs("scratch-repro", "repo", "abcdef123")
	want := []string{"new", "--detach", "--cd", "--repo", "repo", "--ref", "abcdef123", "--", "scratch-repro"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}

	got = tmuxPickDetachedArgs("scratch-base", "repo", "")
	want = []string{"new", "--detach", "--cd", "--repo", "repo", "--", "scratch-base"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args without ref = %v, want %v", got, want)
	}
}

func TestTmuxPickPromoteArgsUsesDetachedName(t *testing.T) {
	item := tmux.PickerItem{
		RepoName:  "repo",
		Branch:    worktree.DetachedBranch,
		Detached:  true,
		WtDirName: "scratch-repro",
	}

	got := tmuxPickPromoteArgs(item, "feature/repro")
	want := []string{"promote", "--repo", "repo", "scratch-repro", "feature/repro"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
}

func TestTmuxPickDeleteArgsDetachedUsesDirName(t *testing.T) {
	item := tmux.PickerItem{
		RepoName:  "repo",
		Branch:    worktree.DetachedBranch,
		Detached:  true,
		WtDirName: "scratch-repro",
	}

	got := tmuxPickDeleteArgs(item)
	want := []string{"rm", "scratch-repro", "--force", "--repo", "repo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
}

func TestMergedDeleteCandidates_SkipsCurrentSessionAndKeepsOrder(t *testing.T) {
	items := []tmux.PickerItem{
		{RepoName: "repo", Branch: "main", WtDirName: "main"},
		{RepoName: "repo", Branch: "feature-a", WtDirName: "feature-a", Merged: true},
		{RepoName: "repo", Branch: "feature-b", WtDirName: "feature-b", Merged: true},
		{RepoName: "repo", Branch: "feature-c", WtDirName: "feature-c", Merged: true},
	}

	got, skippedCurrent := mergedDeleteCandidates(items, "repo/feature-b")
	if !skippedCurrent {
		t.Fatal("expected current session worktree to be skipped")
	}

	want := []string{"feature-a", "feature-c"}
	assertBranches(t, branches(got), want)
}

func TestMergedDeleteCandidates_NoCurrentSessionDeletesAllMerged(t *testing.T) {
	items := []tmux.PickerItem{
		{RepoName: "repo", Branch: "main", WtDirName: "main"},
		{RepoName: "repo", Branch: "feature-a", WtDirName: "feature-a", Merged: true},
		{RepoName: "other", Branch: "feature-b", WtDirName: "feature-b", Merged: true},
	}

	got, skippedCurrent := mergedDeleteCandidates(items, "")
	if skippedCurrent {
		t.Fatal("did not expect any current session skip")
	}

	want := []string{"feature-a", "feature-b"}
	assertBranches(t, branches(got), want)
}

func TestMergedDeleteSkipReasonFromState(t *testing.T) {
	tests := []struct {
		name     string
		children []string
		dirty    bool
		unpushed bool
		want     string
	}{
		{
			name: "safe",
			want: "",
		},
		{
			name:     "stacked children only",
			children: []string{"child-a", "child-b"},
			want:     "stacked children: child-a, child-b",
		},
		{
			name:  "dirty only",
			dirty: true,
			want:  "uncommitted changes",
		},
		{
			name:     "all reasons",
			children: []string{"child-a"},
			dirty:    true,
			unpushed: true,
			want:     "stacked children: child-a; uncommitted changes; unpushed commits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergedDeleteSkipReasonFromState(tt.children, tt.dirty, tt.unpushed)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAvailableExistingBranches(t *testing.T) {
	remoteBranches := []string{"main", "feat-a", "feat-b", "shared"}
	items := []tmux.PickerItem{
		item("repo1", "main"),
		item("repo1", "feat-a"),
		item("repo2", "shared"),
	}

	got := availableExistingBranches(remoteBranches, "repo1", items)
	want := []string{"feat-b", "shared"}
	assertBranches(t, got, want)
}

func TestAvailableExistingBranches_IgnoresDetachedItems(t *testing.T) {
	remoteBranches := []string{"main", "feat-a", "feat-b"}
	items := []tmux.PickerItem{
		item("repo1", "main"),
		{
			RepoName:  "repo1",
			Branch:    "feat-a",
			Detached:  true,
			WtDirName: "scratch-repro",
		},
	}

	got := availableExistingBranches(remoteBranches, "repo1", items)
	want := []string{"feat-a", "feat-b"}
	assertBranches(t, got, want)
}

func TestFindItemByPath_Detached(t *testing.T) {
	items := []tmux.PickerItem{
		{RepoName: "repo", Branch: "main", WtPath: "/fake/repo/main"},
		{RepoName: "repo", Branch: worktree.DetachedBranch, Detached: true, WtDirName: "scratch", WtPath: "/fake/repo/scratch"},
	}

	got := findItemByPath(items, "/fake/repo/scratch")
	if got == nil {
		t.Fatal("expected item")
	}
	if !got.Detached || got.WtDirName != "scratch" {
		t.Fatalf("item = %#v, want detached scratch", got)
	}
	if missing := findItemByPath(items, "/fake/repo/missing"); missing != nil {
		t.Fatalf("missing item = %#v, want nil", missing)
	}
}

func TestExistingBranchCacheRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	want := []string{"main", "feat-a", "feat-b"}
	if err := saveExistingBranchCache("repo1", want); err != nil {
		t.Fatalf("saveExistingBranchCache() error = %v", err)
	}

	got, err := loadExistingBranchCache("repo1")
	if err != nil {
		t.Fatalf("loadExistingBranchCache() error = %v", err)
	}
	assertBranches(t, got, want)
}

func TestExtractBranchFromPRLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "normal PR line",
			line: "#42  Add picker action  (raj)  [feature/tmux-picker]",
			want: "feature/tmux-picker",
		},
		{
			name: "branch with brackets earlier in title",
			line: "#43  Fix [docs] rendering  (raj)  [fix/docs-rendering]",
			want: "fix/docs-rendering",
		},
		{
			name: "missing brackets",
			line: "#44  No branch here",
			want: "",
		},
		{
			name: "empty branch",
			line: "#45  Bad line []",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractBranchFromPRLine(tt.line); got != tt.want {
				t.Fatalf("extractBranchFromPRLine(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestMergedDeleteLabel(t *testing.T) {
	item := tmux.PickerItem{RepoName: "repo-a", Branch: "feature-a"}
	if got := mergedDeleteLabel(item, false); got != "feature-a" {
		t.Fatalf("single-repo label = %q, want feature-a", got)
	}
	if got := mergedDeleteLabel(item, true); got != "repo-a/feature-a" {
		t.Fatalf("multi-repo label = %q, want repo-a/feature-a", got)
	}
}

func TestMergedDeleteHasMultipleRepos(t *testing.T) {
	if mergedDeleteHasMultipleRepos(nil) {
		t.Fatal("empty item set should not count as multiple repos")
	}
	if mergedDeleteHasMultipleRepos([]tmux.PickerItem{item("repo-a", "main"), item("repo-a", "feature")}) {
		t.Fatal("same repo items should not count as multiple repos")
	}
	if !mergedDeleteHasMultipleRepos([]tmux.PickerItem{item("repo-a", "main"), item("repo-b", "feature")}) {
		t.Fatal("different repo items should count as multiple repos")
	}
}

func TestFindItemByPath(t *testing.T) {
	items := []tmux.PickerItem{
		item("repo", "main"),
		item("repo", "feature"),
	}

	got := findItemByPath(items, "/fake/repo/feature")
	if got == nil || got.Branch != "feature" {
		t.Fatalf("findItemByPath() = %+v, want feature item", got)
	}
	if got := findItemByPath(items, "/fake/repo/missing"); got != nil {
		t.Fatalf("findItemByPath() = %+v, want nil for missing path", got)
	}
}

func TestBuildStackChain(t *testing.T) {
	st := makeStack(
		"feature-a", "main",
		"feature-b", "feature-a",
		"feature-c", "feature-b",
		"feature-d", "feature-b",
	)

	got := buildStackChain(st, "feature-b")
	for _, want := range []string{"main", "feature-a", "[feature-b]", "feature-c", "feature-d"} {
		if !strings.Contains(got, want) {
			t.Fatalf("buildStackChain() = %q, missing %q", got, want)
		}
	}
	if !strings.Contains(got, "→") {
		t.Fatalf("buildStackChain() = %q, want arrow separators", got)
	}
}

func TestTmuxInstallCommand_PrintsConfig(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return runApp("tmux", "install")
	})
	if err != nil {
		t.Fatalf("tmux install failed: %v", err)
	}
	for _, want := range []string{"bind w run-shell", "tmux pick", "status-right", "status-interval 3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tmux install output missing %q:\n%s", want, out)
		}
	}
}

func TestTmuxExistingBranchesCommand_ListsAvailableRemoteBranches(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "tmuxbranches"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	worktreeDir := filepath.Join(home, ".willow", "worktrees", "tmuxbranches")
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		t.Fatalf("read worktrees dir: %v", err)
	}
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	wg := &git.Git{Dir: mainDir}
	if _, err := wg.Run("checkout", "-b", "remote-only"); err != nil {
		t.Fatalf("create remote-only branch: %v", err)
	}
	if _, err := wg.Run("push", "origin", "remote-only"); err != nil {
		t.Fatalf("push remote-only branch: %v", err)
	}
	if _, err := wg.Run("checkout", "-"); err != nil {
		t.Fatalf("checkout previous branch: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runApp("tmux", "existing-branches", "--repo", "tmuxbranches", "--refresh")
	})
	if err != nil {
		t.Fatalf("tmux existing-branches failed: %v", err)
	}
	if !strings.Contains(out, "remote-only") {
		t.Fatalf("expected available remote branch in output, got:\n%s", out)
	}
	if strings.Contains(out, entries[0].Name()) {
		t.Fatalf("current worktree branch should be filtered out, got:\n%s", out)
	}
}

func TestTmuxListCommand_PrintsPickerLines(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "tmuxlist"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	worktreeDir := filepath.Join(home, ".willow", "worktrees", "tmuxlist")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	if err := os.Chdir(mainDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := runApp("new", "feature-picker", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runApp("tmux", "list", "--repo", "tmuxlist")
	})
	if err != nil {
		t.Fatalf("tmux list failed: %v", err)
	}
	if !strings.Contains(out, "feature-picker") {
		t.Fatalf("expected picker output to include feature branch, got:\n%s", out)
	}
	if !strings.Contains(out, filepath.Join(".willow", "worktrees", "tmuxlist", "feature-picker")) {
		t.Fatalf("expected picker output to include shortened worktree path, got:\n%s", out)
	}
}

func TestTmuxStatusBarCommand_CountsWorktreesAndAgents(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()

	if err := runApp("clone", origin, "tmuxstatus"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	worktreeDir := filepath.Join(home, ".willow", "worktrees", "tmuxstatus")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	if err := os.Chdir(mainDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := runApp("new", "feature-agent", "--no-fetch"); err != nil {
		t.Fatalf("new failed: %v", err)
	}
	writeActiveSessionFile(t, "tmuxstatus", "feature-agent", "s1", claude.StatusDone)

	out, err := captureStdout(t, func() error {
		return runApp("tmux", "status-bar")
	})
	if err != nil {
		t.Fatalf("tmux status-bar failed: %v", err)
	}
	if !strings.Contains(out, " 2 ") {
		t.Fatalf("expected two worktrees in status bar, got %q", out)
	}
	if !strings.Contains(out, " 1") {
		t.Fatalf("expected one active agent in status bar, got %q", out)
	}
}

func assertBranches(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q (full: got %v, want %v)", i, got[i], want[i], got, want)
		}
	}
}
