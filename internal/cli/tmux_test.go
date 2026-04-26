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

func installFakeTmuxForCLI(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + shellQuote(logPath) + "\n" +
		"case \"$1\" in\n" +
		"  has-session) case \"$3\" in */existing|*/current|exists) exit 0 ;; *) exit 1 ;; esac ;;\n" +
		"  list-panes) printf '%%1\\n%%2\\n' ;;\n" +
		"  list-sessions) printf 'repo/existing\\nrepo/current\\n' ;;\n" +
		"  display-message) printf 'repo/current\\n' ;;\n" +
		"  capture-pane) printf 'pane output\\n' ;;\n" +
		"  *) exit 0 ;;\n" +
		"esac\n"
	writeTestExecutable(t, binDir, "tmux", script)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func installFakeWillowForTmux(t *testing.T, wtRoot string) (string, string) {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "willow.log")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + shellQuote(logPath) + "\n" +
		"last=''\n" +
		"repo='repo'\n" +
		"prev=''\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$prev\" = '--repo' ]; then repo=\"$arg\"; fi\n" +
		"  last=\"$arg\"\n" +
		"  prev=\"$arg\"\n" +
		"done\n" +
		"case \"$1\" in\n" +
		"  new|checkout) path=" + shellQuote(wtRoot) + "/\"$repo\"/\"$last\"; mkdir -p \"$path\"; printf '%s\\n' \"$path\" ;;\n" +
		"  *) exit 0 ;;\n" +
		"esac\n"
	path := writeTestExecutable(t, binDir, "willow-test", script)
	return path, logPath
}

func setupTmuxCommandHome(t *testing.T, repos ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if len(repos) == 0 {
		repos = []string{"repo"}
	}
	for _, repo := range repos {
		if err := os.MkdirAll(filepath.Join(home, ".willow", "repos", repo+".git"), 0o755); err != nil {
			t.Fatalf("mkdir repo %s: %v", repo, err)
		}
		if err := os.MkdirAll(filepath.Join(home, ".willow", "worktrees", repo), 0o755); err != nil {
			t.Fatalf("mkdir worktrees %s: %v", repo, err)
		}
	}
	return home
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

func TestTmuxPickerHeaderActions(t *testing.T) {
	want := "^N new ^T detach ^U promote ^B rebase ^E existing ^P PR ^G dispatch ^S sync ^D rm ^X prune"
	if tmuxPickerHeader != want {
		t.Fatalf("tmux picker header = %q, want %q", tmuxPickerHeader, want)
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

func TestLsShellCompletionListsRepos(t *testing.T) {
	setupTmuxCommandHome(t, "alpha", "beta")
	out, err := captureStdout(t, func() error {
		return runApp("ls", "--generate-shell-completion")
	})
	if err != nil {
		t.Fatalf("ls shell completion failed: %v", err)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("ls completion output = %q, want repo names", out)
	}
}

func TestTmuxPickCommandReturnsWhenNoWorktrees(t *testing.T) {
	setupTmuxCommandHome(t, "empty")
	out, err := captureStderr(t, func() error {
		return runApp("tmux", "pick", "--repo", "empty")
	})
	if err != nil {
		t.Fatalf("tmux pick empty repo should not fail: %v", err)
	}
	if !strings.Contains(out, "No worktrees found.") {
		t.Fatalf("tmux pick stderr = %q, want no-worktrees message", out)
	}
}

func TestTmuxPreviewCommandPrintsMetadataForOfflineSession(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	installFakeTmuxForCLI(t)
	if err := runApp("clone", origin, "tmuxpreview"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	wtDir := firstWorktreeDir(t, filepath.Join(home, ".willow", "worktrees", "tmuxpreview"))
	wtPath := filepath.Join(home, ".willow", "worktrees", "tmuxpreview", wtDir)
	line := tmux.FormatPickerLines([]tmux.PickerItem{
		{RepoName: "tmuxpreview", Branch: wtDir, WtDirName: wtDir, WtPath: wtPath},
	})[0]

	out, err := captureStdout(t, func() error {
		return runApp("tmux", "preview", line)
	})
	if err != nil {
		t.Fatalf("tmux preview failed: %v", err)
	}
	for _, want := range []string{"tmuxpreview/" + wtDir, "Branch:", "Session 'tmuxpreview/" + wtDir + "' is offline"} {
		if !strings.Contains(out, want) {
			t.Fatalf("preview output missing %q:\n%s", want, out)
		}
	}
}

func TestTmuxPickSwitchCreatesAndSwitchesSession(t *testing.T) {
	setupTmuxCommandHome(t, "repo")
	logPath := installFakeTmuxForCLI(t)
	t.Setenv("TMUX", "/tmp/tmux.sock")

	items := []tmux.PickerItem{
		{RepoName: "repo", Branch: "feature", WtDirName: "feature", WtPath: "/work/repo/feature"},
	}
	selection := tmux.FormatPickerLines(items)[0]

	if err := tmuxPickSwitch(selection, items); err != nil {
		t.Fatalf("tmuxPickSwitch: %v", err)
	}

	logText := readTestFile(t, logPath)
	for _, want := range []string{
		"has-session -t repo/feature",
		"new-session -d -s repo/feature -c /work/repo/feature",
		"switch-client -t repo/feature",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("tmux log missing %q:\n%s", want, logText)
		}
	}
	if err := tmuxPickSwitch(selection, nil); err == nil || !strings.Contains(err.Error(), "worktree not found") {
		t.Fatalf("missing item error = %v, want worktree not found", err)
	}
}

func TestTmuxPickerCreateActionsRunWillowAndEnsureSession(t *testing.T) {
	home := setupTmuxCommandHome(t, "repo")
	tmuxLog := installFakeTmuxForCLI(t)
	self, willowLog := installFakeWillowForTmux(t, filepath.Join(home, ".willow", "worktrees"))
	t.Setenv("TMUX", "/tmp/tmux.sock")

	baseItem := tmux.PickerItem{
		RepoName:  "repo",
		Branch:    "main",
		Head:      "abcdef1234567890",
		WtDirName: "main",
		WtPath:    filepath.Join(home, ".willow", "worktrees", "repo", "main"),
	}
	detachedItem := tmux.PickerItem{
		RepoName:  "repo",
		Branch:    worktree.DetachedBranch,
		Head:      "feedface12345678",
		Detached:  true,
		WtDirName: "scratch",
		WtPath:    filepath.Join(home, ".willow", "worktrees", "repo", "scratch"),
	}
	items := []tmux.PickerItem{baseItem, detachedItem}
	lines := tmux.FormatPickerLines(items)

	if err := tmuxPickNew(self, "feature-new", "repo", "", items); err != nil {
		t.Fatalf("tmuxPickNew: %v", err)
	}
	if err := tmuxPickDetached(self, "scratch-copy", lines[0], "", "", items); err != nil {
		t.Fatalf("tmuxPickDetached: %v", err)
	}
	if err := tmuxPickPromote(self, "feature-promoted", lines[1], items); err != nil {
		t.Fatalf("tmuxPickPromote: %v", err)
	}
	if err := tmuxPickSync(self, "repo", "", items, "feature-new"); err != nil {
		t.Fatalf("tmuxPickSync: %v", err)
	}

	willowText := readTestFile(t, willowLog)
	for _, want := range []string{
		"new --cd --repo repo -- feature-new",
		"new --detach --cd --repo repo --ref abcdef1234567890 -- scratch-copy",
		"promote --repo repo scratch feature-promoted",
		"sync --repo repo feature-new",
	} {
		if !strings.Contains(willowText, want) {
			t.Fatalf("willow log missing %q:\n%s", want, willowText)
		}
	}

	tmuxText := readTestFile(t, tmuxLog)
	for _, want := range []string{
		"new-session -d -s repo/feature-new",
		"new-session -d -s repo/scratch-copy",
		"switch-client -t repo/scratch",
	} {
		if !strings.Contains(tmuxText, want) {
			t.Fatalf("tmux log missing %q:\n%s", want, tmuxText)
		}
	}
}

func TestTmuxPickerActionValidationErrors(t *testing.T) {
	setupTmuxCommandHome(t, "repo")
	self, _ := installFakeWillowForTmux(t, filepath.Join(t.TempDir(), "worktrees"))
	item := tmux.PickerItem{RepoName: "repo", Branch: "main", WtDirName: "main", WtPath: "/work/repo/main"}
	selection := tmux.FormatPickerLines([]tmux.PickerItem{item})[0]

	if err := tmuxPickNew(self, "", "repo", "", nil); err == nil || !strings.Contains(err.Error(), "enter a branch name") {
		t.Fatalf("tmuxPickNew empty query error = %v", err)
	}
	if err := tmuxPickDetached(self, "", selection, "repo", "", []tmux.PickerItem{item}); err == nil || !strings.Contains(err.Error(), "detached worktree name") {
		t.Fatalf("tmuxPickDetached empty query error = %v", err)
	}
	if err := tmuxPickPromote(self, "feature", selection, []tmux.PickerItem{item}); err == nil || !strings.Contains(err.Error(), "already on branch") {
		t.Fatalf("tmuxPickPromote branch error = %v", err)
	}
	if err := tmuxPickDispatch(self, "", "repo", "", nil); err == nil || !strings.Contains(err.Error(), "type a prompt first") {
		t.Fatalf("tmuxPickDispatch empty prompt error = %v", err)
	}
	if err := tmuxPickNewWithBase(self, "", "repo", "", nil); err == nil || !strings.Contains(err.Error(), "enter a branch name") {
		t.Fatalf("tmuxPickNewWithBase empty prompt error = %v", err)
	}
}

func TestTmuxPickExistingUsesCachedBranchesAndSelectedQuery(t *testing.T) {
	home := setupTmuxCommandHome(t, "repo")
	installFakeTmuxForCLI(t)
	self, willowLog := installFakeWillowForTmux(t, filepath.Join(home, ".willow", "worktrees"))
	if err := saveExistingBranchCache("repo", []string{"main", "remote-only"}); err != nil {
		t.Fatalf("save cache: %v", err)
	}
	t.Setenv("FZF_DEFAULT_OPTS", "--filter=remote-only")

	items := []tmux.PickerItem{{RepoName: "repo", Branch: "main", WtDirName: "main", WtPath: filepath.Join(home, ".willow", "worktrees", "repo", "main")}}
	if err := tmuxPickExisting(self, "repo", "", items, "remote"); err != nil {
		t.Fatalf("tmuxPickExisting: %v", err)
	}

	willowText := readTestFile(t, willowLog)
	if !strings.Contains(willowText, "new -e --cd --repo repo -- remote-only") {
		t.Fatalf("willow log missing checkout of cached branch:\n%s", willowText)
	}
}

func TestTmuxPickNewWithBaseUsesSelectedWorktreeBranch(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	installFakeTmuxForCLI(t)
	self, willowLog := installFakeWillowForTmux(t, filepath.Join(home, ".willow", "worktrees"))

	if err := runApp("clone", origin, "tmuxbase"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	worktreeDir := filepath.Join(home, ".willow", "worktrees", "tmuxbase")
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		t.Fatalf("read worktrees dir: %v", err)
	}
	if err := os.Chdir(filepath.Join(worktreeDir, entries[0].Name())); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := runApp("new", "feature-base", "--no-fetch"); err != nil {
		t.Fatalf("new feature-base failed: %v", err)
	}
	t.Setenv("FZF_DEFAULT_OPTS", "--filter=feature-base")

	if err := tmuxPickNewWithBase(self, "feature-child", "tmuxbase", "", nil); err != nil {
		t.Fatalf("tmuxPickNewWithBase: %v", err)
	}

	willowText := readTestFile(t, willowLog)
	if !strings.Contains(willowText, "new --base feature-base --cd --repo tmuxbase -- feature-child") {
		t.Fatalf("willow log missing base new:\n%s", willowText)
	}
}

func TestTmuxPickPRChecksOutSelectedBranch(t *testing.T) {
	home := setupTmuxCommandHome(t, "repo")
	installFakeTmuxForCLI(t)
	self, willowLog := installFakeWillowForTmux(t, filepath.Join(home, ".willow", "worktrees"))
	binDir := t.TempDir()
	writeTestExecutable(t, binDir, "gh", "#!/bin/sh\nprintf '#42  Fix picker  (raj)  [feature-pr]\\n'\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FZF_DEFAULT_OPTS", "--filter=feature-pr")

	if err := tmuxPickPR(self, "repo", "", nil); err != nil {
		t.Fatalf("tmuxPickPR: %v", err)
	}
	willowText := readTestFile(t, willowLog)
	if !strings.Contains(willowText, "checkout --cd --repo repo -- feature-pr") {
		t.Fatalf("willow log missing PR checkout:\n%s", willowText)
	}
}

func TestTmuxPickPRReportsMissingCLIAndNoOpenPRs(t *testing.T) {
	setupTmuxCommandHome(t, "repo")
	t.Setenv("PATH", t.TempDir())
	if err := tmuxPickPR("willow", "repo", "", nil); err == nil || !strings.Contains(err.Error(), "gh CLI is required") {
		t.Fatalf("tmuxPickPR missing gh error = %v", err)
	}

	binDir := t.TempDir()
	writeTestExecutable(t, binDir, "gh", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", binDir)
	if err := tmuxPickPR("willow", "repo", "", nil); err == nil || !strings.Contains(err.Error(), "no open PRs found") {
		t.Fatalf("tmuxPickPR empty list error = %v", err)
	}
}

func TestTmuxSwCommandSwitchesToWorktreeSession(t *testing.T) {
	home := setupTmuxCommandHome(t, "repo")
	tmuxLog := installFakeTmuxForCLI(t)
	t.Setenv("TMUX", "/tmp/tmux.sock")
	wtPath := filepath.Join(home, ".willow", "worktrees", "repo", "feature")

	if err := runApp("tmux", "sw", wtPath); err != nil {
		t.Fatalf("tmux sw failed: %v", err)
	}
	tmuxText := readTestFile(t, tmuxLog)
	for _, want := range []string{
		"has-session -t repo/feature",
		"new-session -d -s repo/feature",
		"switch-client -t repo/feature",
	} {
		if !strings.Contains(tmuxText, want) {
			t.Fatalf("tmux log missing %q:\n%s", want, tmuxText)
		}
	}
}

func TestTmuxPickDispatchWritesPromptAndStartsClaude(t *testing.T) {
	home := setupTmuxCommandHome(t, "repo")
	tmuxLog := installFakeTmuxForCLI(t)
	self, willowLog := installFakeWillowForTmux(t, filepath.Join(home, ".willow", "worktrees"))
	t.Setenv("TMUX", "/tmp/tmux.sock")

	if err := tmuxPickDispatch(self, "Fix a gnarly bug", "repo", "", nil); err != nil {
		t.Fatalf("tmuxPickDispatch: %v", err)
	}

	willowText := readTestFile(t, willowLog)
	if !strings.Contains(willowText, "new --cd --repo repo -- dispatch--fix-a-gnarly-bug") {
		t.Fatalf("willow log missing dispatch new:\n%s", willowText)
	}
	promptPath := filepath.Join(home, ".willow", "prompts", "repo", "dispatch--fix-a-gnarly-bug.prompt")
	if got := strings.TrimSpace(readTestFile(t, promptPath)); got != "Fix a gnarly bug" {
		t.Fatalf("prompt file = %q, want original prompt", got)
	}
	tmuxText := readTestFile(t, tmuxLog)
	for _, want := range []string{
		"new-session -d -s repo/dispatch--fix-a-gnarly-bug",
		"send-keys -t repo/dispatch--fix-a-gnarly-bug",
		"claude \"$(cat",
	} {
		if !strings.Contains(tmuxText, want) {
			t.Fatalf("tmux log missing %q:\n%s", want, tmuxText)
		}
	}
}

func TestTmuxPickDeleteKillsSessionAndRunsWillowRm(t *testing.T) {
	setupTmuxCommandHome(t, "repo")
	tmuxLog := installFakeTmuxForCLI(t)
	self, willowLog := installFakeWillowForTmux(t, filepath.Join(t.TempDir(), "worktrees"))

	item := tmux.PickerItem{RepoName: "repo", Branch: "existing", WtDirName: "existing", WtPath: "/work/repo/existing"}
	selection := tmux.FormatPickerLines([]tmux.PickerItem{item})[0]
	if err := tmuxPickDelete(self, selection, []tmux.PickerItem{item}); err != nil {
		t.Fatalf("tmuxPickDelete: %v", err)
	}

	if tmuxText := readTestFile(t, tmuxLog); !strings.Contains(tmuxText, "kill-session -t repo/existing") {
		t.Fatalf("tmux log missing kill-session:\n%s", tmuxText)
	}
	if willowText := readTestFile(t, willowLog); !strings.Contains(willowText, "rm existing --force --repo repo") {
		t.Fatalf("willow log missing rm:\n%s", willowText)
	}
}

func TestTmuxPickDeleteMergedReportsNoCandidates(t *testing.T) {
	self, _ := installFakeWillowForTmux(t, filepath.Join(t.TempDir(), "worktrees"))
	if err := tmuxPickDeleteMerged(self, "", nil); err == nil || !strings.Contains(err.Error(), "no merged worktrees") {
		t.Fatalf("tmuxPickDeleteMerged empty error = %v", err)
	}
	items := []tmux.PickerItem{{RepoName: "repo", Branch: "merged", WtDirName: "merged", Merged: true}}
	if err := tmuxPickDeleteMerged(self, "repo/merged", items); err == nil || !strings.Contains(err.Error(), "current session skipped") {
		t.Fatalf("tmuxPickDeleteMerged current-only error = %v", err)
	}
}

func TestTmuxPickDeleteMergedConfirmsAndDeletesSafeCandidates(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	installFakeTmuxForCLI(t)
	self, willowLog := installFakeWillowForTmux(t, filepath.Join(home, ".willow", "worktrees"))

	if err := runApp("clone", origin, "mergeddelete"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	worktreeRoot := filepath.Join(home, ".willow", "worktrees", "mergeddelete")
	baseDir := filepath.Join(worktreeRoot, firstWorktreeDir(t, worktreeRoot))
	if err := os.Chdir(baseDir); err != nil {
		t.Fatalf("chdir base: %v", err)
	}
	if err := runApp("new", "safe-merged", "--no-fetch"); err != nil {
		t.Fatalf("new safe-merged failed: %v", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdin: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = r.Close()
	})
	if _, err := w.WriteString("y\n"); err != nil {
		t.Fatalf("write confirmation: %v", err)
	}
	_ = w.Close()

	item := tmux.PickerItem{
		RepoName:  "mergeddelete",
		Branch:    "safe-merged",
		WtDirName: "safe-merged",
		WtPath:    filepath.Join(worktreeRoot, "safe-merged"),
		Merged:    true,
	}
	if err := tmuxPickDeleteMerged(self, "", []tmux.PickerItem{item}); err != nil {
		t.Fatalf("tmuxPickDeleteMerged: %v", err)
	}
	os.Stdin = origStdin

	if willowText := readTestFile(t, willowLog); !strings.Contains(willowText, "rm safe-merged --force --repo mergeddelete") {
		t.Fatalf("willow log missing merged delete rm:\n%s", willowText)
	}
}

func TestResolveRepoSingleAndNoRepos(t *testing.T) {
	setupTmuxCommandHome(t, "solo")
	if got, err := resolveRepo("", "", nil); err != nil || got != "solo" {
		t.Fatalf("resolveRepo single = %q, %v; want solo, nil", got, err)
	}
	if got, err := resolveRepo("explicit", "", nil); err != nil || got != "explicit" {
		t.Fatalf("resolveRepo explicit = %q, %v; want explicit, nil", got, err)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := resolveRepo("", "", nil); err == nil || !strings.Contains(err.Error(), "no repos found") {
		t.Fatalf("resolveRepo no repos error = %v, want no repos found", err)
	}
}

func TestResolveRepoOrdersCurrentAndActiveRepos(t *testing.T) {
	setupTmuxCommandHome(t, "alpha", "beta", "gamma")
	t.Setenv("FZF_DEFAULT_OPTS", "--filter=gamma")
	items := []tmux.PickerItem{{RepoName: "gamma", Status: claude.StatusBusy}}

	if got, err := resolveRepo("", "beta/current", items); err != nil || got != "gamma" {
		t.Fatalf("resolveRepo multi = %q, %v; want gamma, nil", got, err)
	}
}

func TestReadTrimmedStdinLine(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = r.Close()
	})
	if _, err := w.WriteString("  y  \n"); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	_ = w.Close()

	if got := readTrimmedStdinLine(); got != "y" {
		t.Fatalf("readTrimmedStdinLine() = %q, want y", got)
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

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func firstWorktreeDir(t *testing.T, worktreeRoot string) string {
	t.Helper()
	entries, err := os.ReadDir(worktreeRoot)
	if err != nil {
		t.Fatalf("read worktree root: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("no worktree dirs in %s", worktreeRoot)
	}
	return entries[0].Name()
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
