package cli

import (
	"testing"

	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/tmux"
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
