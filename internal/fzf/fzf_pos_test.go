package fzf

import (
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
