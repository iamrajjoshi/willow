package fzf

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	fzflib "github.com/junegunn/fzf/src"
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

func TestRunReturnsSelectionAndHandlesCancel(t *testing.T) {
	oldRunFzf := runFzf
	t.Cleanup(func() { runFzf = oldRunFzf })

	runFzf = func(lines []string, extraArgs []string, cfg *config) ([]string, int, error) {
		if !reflect.DeepEqual(lines, []string{"one", "two"}) {
			t.Fatalf("lines = %v, want [one two]", lines)
		}
		if !reflect.DeepEqual(extraArgs, []string{"--no-multi"}) {
			t.Fatalf("extraArgs = %v, want --no-multi", extraArgs)
		}
		return []string{"two"}, 0, nil
	}
	got, err := Run([]string{"one", "two"})
	if err != nil || got != "two" {
		t.Fatalf("Run() = %q, %v; want two, nil", got, err)
	}

	runFzf = func([]string, []string, *config) ([]string, int, error) {
		return nil, fzflib.ExitInterrupt, nil
	}
	got, err = Run([]string{"one"})
	if err != nil || got != "" {
		t.Fatalf("Run interrupt = %q, %v; want empty, nil", got, err)
	}

	runFzf = func([]string, []string, *config) ([]string, int, error) {
		return nil, fzflib.ExitNoMatch, nil
	}
	got, err = Run([]string{"one"})
	if err != nil || got != "" {
		t.Fatalf("Run no match = %q, %v; want empty, nil", got, err)
	}
}

func TestRunWrapsErrorsAndEmptyOutput(t *testing.T) {
	oldRunFzf := runFzf
	t.Cleanup(func() { runFzf = oldRunFzf })

	runFzf = func([]string, []string, *config) ([]string, int, error) {
		return nil, 0, errors.New("boom")
	}
	if _, err := Run([]string{"one"}); err == nil || !strings.Contains(err.Error(), "fzf failed") {
		t.Fatalf("Run error = %v, want wrapped fzf error", err)
	}

	runFzf = func([]string, []string, *config) ([]string, int, error) {
		return nil, 0, nil
	}
	got, err := Run([]string{"one"})
	if err != nil || got != "" {
		t.Fatalf("Run empty output = %q, %v; want empty, nil", got, err)
	}
}

func TestRunExpectParsesQueryKeyAndSelection(t *testing.T) {
	oldRunFzf := runFzf
	t.Cleanup(func() { runFzf = oldRunFzf })

	runFzf = func(lines []string, extraArgs []string, cfg *config) ([]string, int, error) {
		if !reflect.DeepEqual(extraArgs, []string{"--no-multi"}) {
			t.Fatalf("extraArgs = %v, want --no-multi", extraArgs)
		}
		return []string{"feature", "ctrl-n", "selected"}, 0, nil
	}

	got, err := RunExpect([]string{"selected"}, WithPrintQuery(), WithExpectKeys("ctrl-n"))
	if err != nil {
		t.Fatalf("RunExpect: %v", err)
	}
	if got.Query != "feature" || got.Key != "ctrl-n" || got.Selection != "selected" {
		t.Fatalf("RunExpect() = %+v, want parsed query/key/selection", got)
	}
}

func TestRunExpectHandlesCancelAndErrors(t *testing.T) {
	oldRunFzf := runFzf
	t.Cleanup(func() { runFzf = oldRunFzf })

	runFzf = func([]string, []string, *config) ([]string, int, error) {
		return nil, fzflib.ExitInterrupt, nil
	}
	got, err := RunExpect([]string{"one"})
	if err != nil || got != nil {
		t.Fatalf("RunExpect interrupt = %+v, %v; want nil, nil", got, err)
	}

	runFzf = func([]string, []string, *config) ([]string, int, error) {
		return nil, 0, errors.New("boom")
	}
	got, err = RunExpect([]string{"one"})
	if err == nil || !strings.Contains(err.Error(), "fzf failed") || got != nil {
		t.Fatalf("RunExpect error = %+v, %v; want wrapped error", got, err)
	}
}

func TestRunMultiReturnsSelectionsAndHandlesEmpty(t *testing.T) {
	oldRunFzf := runFzf
	t.Cleanup(func() { runFzf = oldRunFzf })

	runFzf = func(lines []string, extraArgs []string, cfg *config) ([]string, int, error) {
		wantArgs := []string{"--multi", "--bind=ctrl-a:select-all"}
		if !reflect.DeepEqual(extraArgs, wantArgs) {
			t.Fatalf("extraArgs = %v, want %v", extraArgs, wantArgs)
		}
		return []string{"one", "two"}, 0, nil
	}
	got, err := RunMulti([]string{"one", "two"})
	if err != nil || !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("RunMulti() = %v, %v; want selections, nil", got, err)
	}

	runFzf = func([]string, []string, *config) ([]string, int, error) {
		return nil, fzflib.ExitNoMatch, nil
	}
	got, err = RunMulti([]string{"one"})
	if err != nil || got != nil {
		t.Fatalf("RunMulti no match = %v, %v; want nil, nil", got, err)
	}

	runFzf = func([]string, []string, *config) ([]string, int, error) {
		return nil, 0, nil
	}
	got, err = RunMulti([]string{"one"})
	if err != nil || got != nil {
		t.Fatalf("RunMulti empty = %v, %v; want nil, nil", got, err)
	}
}

func TestRunMultiWrapsErrors(t *testing.T) {
	oldRunFzf := runFzf
	t.Cleanup(func() { runFzf = oldRunFzf })

	runFzf = func([]string, []string, *config) ([]string, int, error) {
		return nil, 0, errors.New("boom")
	}
	got, err := RunMulti([]string{"one"})
	if err == nil || !strings.Contains(err.Error(), "fzf failed") || got != nil {
		t.Fatalf("RunMulti error = %v, %v; want wrapped error", got, err)
	}
}

func TestRunFzfWithLibReturnsParseError(t *testing.T) {
	_, _, err := runFzfWithLib([]string{"one"}, []string{"--definitely-not-a-real-fzf-option"}, defaults())
	if err == nil {
		t.Fatal("runFzfWithLib should return parse error for invalid option")
	}
	if !strings.Contains(err.Error(), "fzf:") {
		t.Fatalf("error = %v, want fzf parse context", err)
	}
}
