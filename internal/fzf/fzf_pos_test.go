package fzf

import (
	"reflect"
	"testing"
)

// TestPosBinding verifies that pos() is correctly included in the fzf args
// when used via WithBind. This doesn't test fzf's runtime behavior but ensures
// the arg construction is correct.
func TestPosBinding(t *testing.T) {
	cfg := defaults()
	WithAnsi()(cfg)
	WithReverse()(cfg)
	WithNoSort()(cfg)
	WithBind("start:reload-sync(echo hello)+pos(3)")(cfg)

	args := buildArgs(cfg)

	found := false
	for i, arg := range args {
		if arg == "--bind" && i+1 < len(args) {
			if args[i+1] == "start:reload-sync(echo hello)+pos(3)" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected --bind with pos() in args, got: %v", args)
	}
}

// TestQueryArg verifies --query is included when WithQuery is set.
func TestQueryArg(t *testing.T) {
	cfg := defaults()
	WithQuery("my-branch")(cfg)

	args := buildArgs(cfg)

	found := false
	for i, arg := range args {
		if arg == "--query" && i+1 < len(args) && args[i+1] == "my-branch" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --query my-branch in args, got: %v", args)
	}
}

// TestQueryEmpty verifies --query is NOT included when WithQuery is empty.
func TestQueryEmpty(t *testing.T) {
	cfg := defaults()
	WithQuery("")(cfg)

	args := buildArgs(cfg)

	for _, arg := range args {
		if arg == "--query" {
			t.Errorf("expected no --query in args when empty, got: %v", args)
		}
	}
}

func TestBuildArgsIncludesAllOptionsInOrder(t *testing.T) {
	cfg := defaults()
	WithAnsi()(cfg)
	WithReverse()(cfg)
	WithNoSort()(cfg)
	WithHeader("Pick one")(cfg)
	WithPreview("willow tmux preview -- {}", "right:50%:wrap")(cfg)
	WithExpectKeys("ctrl-n", "ctrl-p")(cfg)
	WithPrintQuery()(cfg)
	WithQuery("feature")(cfg)
	WithBind("start:reload(echo hi)", "ctrl-r:reload-sync(echo refresh)")(cfg)
	WithDelimiter("\\|")(cfg)
	WithNth("1,2")(cfg)

	got := buildArgs(cfg)
	want := []string{
		"--ansi",
		"--reverse",
		"--no-sort",
		"--cycle",
		"--header", "Pick one",
		"--preview", "willow tmux preview -- {}",
		"--preview-window", "right:50%:wrap",
		"--expect", "ctrl-n,ctrl-p",
		"--print-query",
		"--query", "feature",
		"--bind", "start:reload(echo hi)",
		"--bind", "ctrl-r:reload-sync(echo refresh)",
		"--delimiter", "\\|",
		"--nth", "1,2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildArgs() = %#v, want %#v", got, want)
	}
}
