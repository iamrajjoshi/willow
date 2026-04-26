package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/gh"
	"github.com/iamrajjoshi/willow/internal/ui"
)

func TestStackCmd(t *testing.T) {
	cmd := stackCmd()
	if cmd.Name != "stack" {
		t.Errorf("Name = %q, want %q", cmd.Name, "stack")
	}
	if len(cmd.Commands) == 0 {
		t.Fatal("expected subcommands, got none")
	}

	var found bool
	for _, sub := range cmd.Commands {
		if sub.Name == "status" {
			found = true
			if len(sub.Aliases) == 0 || sub.Aliases[0] != "s" {
				t.Errorf("status aliases = %v, want [s]", sub.Aliases)
			}
			break
		}
	}
	if !found {
		t.Error("expected 'status' subcommand")
	}
}

func TestFormatPRAnnotation(t *testing.T) {
	u := &ui.UI{}
	pr := &gh.PRInfo{
		Number:       42,
		ReviewStatus: "APPROVED",
		Mergeable:    "MERGEABLE",
		Additions:    3,
		Deletions:    1,
	}

	got := formatPRAnnotation(u, pr)
	for _, want := range []string{"#42", "CI", "Review", "MERGEABLE", "+3", "-1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatPRAnnotation() missing %q: %q", want, got)
		}
	}
}

func TestFindGHDir(t *testing.T) {
	root := t.TempDir()
	if _, err := findGHDir(root); err == nil {
		t.Fatal("expected empty worktree dir to fail")
	}

	want := filepath.Join(root, "main")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	got, err := findGHDir(root)
	if err != nil {
		t.Fatalf("findGHDir() error = %v", err)
	}
	if got != want {
		t.Fatalf("findGHDir() = %q, want %q", got, want)
	}
}

func TestStackStatusJSON(t *testing.T) {
	origin := setupTestEnv(t)
	home, _ := os.UserHomeDir()
	if err := runApp("clone", origin, "stackstatus"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	worktreeDir := filepath.Join(home, ".willow", "worktrees", "stackstatus")
	entries, _ := os.ReadDir(worktreeDir)
	mainDir := filepath.Join(worktreeDir, entries[0].Name())
	if err := os.Chdir(mainDir); err != nil {
		t.Fatalf("chdir main: %v", err)
	}
	if err := runApp("new", "feature-a", "--no-fetch"); err != nil {
		t.Fatalf("new feature-a failed: %v", err)
	}
	if err := runApp("new", "feature-b", "--base", "feature-a", "--no-fetch"); err != nil {
		t.Fatalf("new feature-b failed: %v", err)
	}

	ghScript := `#!/bin/sh
set -eu
if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  cat <<'JSON'
[
  {"number":10,"title":"Feature A","headRefName":"feature-a","headRefOid":"sha-a","baseRefName":"main","state":"OPEN","reviewDecision":"APPROVED","mergeable":"MERGEABLE","additions":5,"deletions":1,"url":"https://github.com/test/repo/pull/10","statusCheckRollup":[{"name":"test","status":"COMPLETED","conclusion":"SUCCESS"}]},
  {"number":11,"title":"Feature B","headRefName":"feature-b","headRefOid":"sha-b","baseRefName":"feature-a","state":"OPEN","reviewDecision":"REVIEW_REQUIRED","mergeable":"UNKNOWN","additions":7,"deletions":2,"url":"https://github.com/test/repo/pull/11","statusCheckRollup":[]}
]
JSON
  exit 0
fi
exit 1
`
	installTestCLIPath(t, ghScript)

	out, err := captureStdout(t, func() error {
		return runApp("stack", "status", "--repo", "stackstatus", "--json")
	})
	if err != nil {
		t.Fatalf("stack status --json failed: %v", err)
	}

	var got map[string]gh.PRInfo
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stack status JSON failed to parse: %v\n%s", err, out)
	}
	if got["feature-a"].Number != 10 {
		t.Fatalf("feature-a PR = %+v, want #10", got["feature-a"])
	}
	if got["feature-b"].BaseRefName != "feature-a" {
		t.Fatalf("feature-b PR = %+v, want base feature-a", got["feature-b"])
	}

	out, err = captureStdout(t, func() error {
		return runApp("stack", "status", "--repo", "stackstatus")
	})
	if err != nil {
		t.Fatalf("stack status failed: %v", err)
	}
	for _, want := range []string{"feature-a", "feature-b", "#10", "#11", "CI", "Review"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stack status output missing %q:\n%s", want, out)
		}
	}
}
