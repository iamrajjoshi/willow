package gh

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCIStatus(t *testing.T) {
	tests := []struct {
		name   string
		checks []checkRun
		want   string
	}{
		{
			name:   "no checks",
			checks: nil,
			want:   "none",
		},
		{
			name: "all pass",
			checks: []checkRun{
				{Name: "test", Status: "COMPLETED", Conclusion: "SUCCESS"},
				{Name: "lint", Status: "COMPLETED", Conclusion: "SUCCESS"},
			},
			want: "pass",
		},
		{
			name: "one failure",
			checks: []checkRun{
				{Name: "test", Status: "COMPLETED", Conclusion: "SUCCESS"},
				{Name: "lint", Status: "COMPLETED", Conclusion: "FAILURE"},
			},
			want: "fail",
		},
		{
			name: "one pending",
			checks: []checkRun{
				{Name: "test", Status: "COMPLETED", Conclusion: "SUCCESS"},
				{Name: "deploy", Status: "IN_PROGRESS", Conclusion: ""},
			},
			want: "pending",
		},
		{
			name: "queued",
			checks: []checkRun{
				{Name: "test", Status: "QUEUED", Conclusion: ""},
			},
			want: "pending",
		},
		{
			name: "failure takes priority over pending",
			checks: []checkRun{
				{Name: "test", Status: "IN_PROGRESS", Conclusion: ""},
				{Name: "lint", Status: "COMPLETED", Conclusion: "FAILURE"},
			},
			want: "fail",
		},
		{
			name: "skipped and neutral count as pass",
			checks: []checkRun{
				{Name: "test", Status: "COMPLETED", Conclusion: "SUCCESS"},
				{Name: "optional", Status: "COMPLETED", Conclusion: "SKIPPED"},
				{Name: "neutral", Status: "COMPLETED", Conclusion: "NEUTRAL"},
			},
			want: "pass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &PRInfo{Checks: tt.checks}
			got := pr.CIStatus()
			if got != tt.want {
				t.Errorf("CIStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGHPRJSONParsing(t *testing.T) {
	raw := `[
		{
			"number": 42,
			"title": "Add feature X",
			"headRefName": "feature-x",
			"state": "OPEN",
			"reviewDecision": "APPROVED",
			"mergeable": "MERGEABLE",
			"additions": 100,
			"deletions": 20,
			"url": "https://github.com/org/repo/pull/42",
			"statusCheckRollup": [
				{"name": "test", "status": "COMPLETED", "conclusion": "SUCCESS"},
				{"name": "lint", "status": "COMPLETED", "conclusion": "SUCCESS"}
			]
		},
		{
			"number": 43,
			"title": "Fix bug Y",
			"headRefName": "fix-y",
			"state": "MERGED",
			"reviewDecision": "",
			"mergeable": "UNKNOWN",
			"additions": 5,
			"deletions": 3,
			"url": "https://github.com/org/repo/pull/43",
			"statusCheckRollup": []
		}
	]`

	var prs []ghPR
	if err := json.Unmarshal([]byte(raw), &prs); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if len(prs) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(prs))
	}

	pr := prs[0]
	if pr.Number != 42 {
		t.Errorf("Number = %d, want 42", pr.Number)
	}
	if pr.Branch != "feature-x" {
		t.Errorf("Branch = %q, want %q", pr.Branch, "feature-x")
	}
	if pr.State != "OPEN" {
		t.Errorf("State = %q, want %q", pr.State, "OPEN")
	}
	if pr.ReviewStatus != "APPROVED" {
		t.Errorf("ReviewStatus = %q, want %q", pr.ReviewStatus, "APPROVED")
	}
	if pr.Mergeable != "MERGEABLE" {
		t.Errorf("Mergeable = %q, want %q", pr.Mergeable, "MERGEABLE")
	}
	if len(pr.Checks) != 2 {
		t.Errorf("Checks count = %d, want 2", len(pr.Checks))
	}
	if pr.Checks[0].Name != "test" || pr.Checks[0].Conclusion != "SUCCESS" {
		t.Errorf("first check = %+v, want test/SUCCESS", pr.Checks[0])
	}

	pr2 := prs[1]
	if pr2.State != "MERGED" {
		t.Errorf("PR2 State = %q, want %q", pr2.State, "MERGED")
	}
	if len(pr2.Checks) != 0 {
		t.Errorf("PR2 Checks count = %d, want 0", len(pr2.Checks))
	}
}

func TestPRLookupArgs(t *testing.T) {
	got := prLookupArgs("feature-x")
	want := []string{
		"pr", "list",
		"--head", "feature-x",
		"--state", "open",
		"--json", prJSONFields,
		"--limit", "20",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prLookupArgs() = %v, want %v", got, want)
	}
}

func TestPRCreateArgs(t *testing.T) {
	tests := []struct {
		name  string
		draft bool
		want  []string
	}{
		{
			name:  "ready for review",
			draft: false,
			want:  []string{"pr", "create", "--fill", "--base", "main", "--head", "feature-x"},
		},
		{
			name:  "draft",
			draft: true,
			want:  []string{"pr", "create", "--fill", "--base", "main", "--head", "feature-x", "--draft"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prCreateArgs("main", "feature-x", tt.draft)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("prCreateArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParsePRListOutput(t *testing.T) {
	raw := `[
		{
			"number": 42,
			"title": "Add feature X",
			"headRefName": "feature-x",
			"state": "OPEN",
			"reviewDecision": "APPROVED",
			"mergeable": "MERGEABLE",
			"additions": 100,
			"deletions": 20,
			"url": "https://github.com/org/repo/pull/42",
			"statusCheckRollup": [
				{"name": "test", "status": "COMPLETED", "conclusion": "SUCCESS"}
			]
		}
	]`

	prs, err := parsePRListOutput([]byte(raw))
	if err != nil {
		t.Fatalf("parsePRListOutput() error = %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}

	pr := prs[0]
	if pr.Number != 42 {
		t.Errorf("Number = %d, want 42", pr.Number)
	}
	if pr.Branch != "feature-x" {
		t.Errorf("Branch = %q, want feature-x", pr.Branch)
	}
	if pr.URL != "https://github.com/org/repo/pull/42" {
		t.Errorf("URL = %q, want pull URL", pr.URL)
	}
	if len(pr.Checks) != 1 || pr.Checks[0].Name != "test" {
		t.Errorf("Checks = %+v, want one test check", pr.Checks)
	}
}

func TestSelectMatchingPR(t *testing.T) {
	prs := []*PRInfo{
		{Number: 41, Branch: "feature-x", HeadRefOID: "sha-other"},
		{Number: 42, Branch: "feature-x", HeadRefOID: "sha-match"},
	}

	if got := selectMatchingPR(prs, "sha-match"); got == nil || got.Number != 42 {
		t.Fatalf("selectMatchingPR() = %+v, want PR #42", got)
	}
	if got := selectMatchingPR(prs, "sha-missing"); got != nil {
		t.Fatalf("selectMatchingPR() = %+v, want nil", got)
	}
	if got := selectMatchingPR([]*PRInfo{{Number: 99}}, ""); got == nil || got.Number != 99 {
		t.Fatalf("selectMatchingPR() with single fallback = %+v, want PR #99", got)
	}
	if got := selectMatchingPR(prs, ""); got != nil {
		t.Fatalf("selectMatchingPR() with ambiguous empty head = %+v, want nil", got)
	}
}

func TestFindOpenPRRunsGHAndSelectsMatchingHead(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "gh.log")
	ghPath := filepath.Join(binDir, "gh")
	script := `#!/bin/sh
set -eu
printf 'cwd=%s\n' "$PWD" >> "` + logPath + `"
printf 'args=%s\n' "$*" >> "` + logPath + `"
/bin/cat <<'JSON'
[
  {
    "number": 42,
    "title": "Feature",
    "headRefName": "feature-a",
    "headRefOid": "sha-match",
    "baseRefName": "main",
    "state": "OPEN",
    "reviewDecision": "APPROVED",
    "mergeable": "MERGEABLE",
    "additions": 4,
    "deletions": 2,
    "url": "https://github.com/org/repo/pull/42",
    "statusCheckRollup": []
  },
  {
    "number": 43,
    "title": "Other",
    "headRefName": "feature-a",
    "headRefOid": "sha-other",
    "baseRefName": "main",
    "state": "OPEN",
    "reviewDecision": "",
    "mergeable": "UNKNOWN",
    "additions": 1,
    "deletions": 1,
    "url": "https://github.com/org/repo/pull/43",
    "statusCheckRollup": []
  }
]
JSON
`
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write gh stub: %v", err)
	}
	t.Setenv("PATH", binDir)

	dir := t.TempDir()
	got, err := FindOpenPR(dir, "feature-a", "sha-match")
	if err != nil {
		t.Fatalf("FindOpenPR() error = %v", err)
	}
	if got == nil || got.Number != 42 || got.HeadRefOID != "sha-match" {
		t.Fatalf("FindOpenPR() = %+v, want PR #42", got)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read gh log: %v", err)
	}
	wantArgs := "args=pr list --head feature-a --state open --json " + prJSONFields + " --limit 20\n"
	if !strings.HasSuffix(string(logData), "\n"+wantArgs) {
		t.Fatalf("unexpected gh invocation:\n%s", logData)
	}
}

func TestFindOpenPRReturnsCommandOutputOnFailure(t *testing.T) {
	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	script := "#!/bin/sh\nprintf 'no auth\\n' >&2\nexit 7\n"
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write gh stub: %v", err)
	}
	t.Setenv("PATH", binDir)

	got, err := FindOpenPR(t.TempDir(), "feature-a", "")
	if err == nil || got != nil {
		t.Fatalf("FindOpenPR failure = %+v, %v; want nil error", got, err)
	}
	if err.Error() != "gh pr list failed: no auth" {
		t.Fatalf("FindOpenPR error = %v, want gh output", err)
	}
}

func TestCreatePRRunsGHAndReturnsURL(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "gh.log")
	ghPath := filepath.Join(binDir, "gh")
	script := `#!/bin/sh
set -eu
printf 'cwd=%s\n' "$PWD" >> "` + logPath + `"
printf 'args=%s\n' "$*" >> "` + logPath + `"
printf 'https://github.com/org/repo/pull/99\n'
`
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write gh stub: %v", err)
	}
	t.Setenv("PATH", binDir)

	dir := t.TempDir()
	got, err := CreatePR(dir, "main", "feature-a", true)
	if err != nil {
		t.Fatalf("CreatePR() error = %v", err)
	}
	if got != "https://github.com/org/repo/pull/99" {
		t.Fatalf("CreatePR() = %q, want PR URL", got)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read gh log: %v", err)
	}
	if !strings.HasSuffix(string(logData), "\nargs=pr create --fill --base main --head feature-a --draft\n") {
		t.Fatalf("unexpected gh invocation:\n%s", logData)
	}
}

func TestBatchPRInfoUsesSingleGHCallAndFiltersBranches(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "gh.log")
	ghPath := filepath.Join(binDir, "gh")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "` + logPath + `"
/bin/cat <<'JSON'
[
  {
    "number": 1,
    "title": "Feature A",
    "headRefName": "feature-a",
    "headRefOid": "sha-a",
    "baseRefName": "main",
    "state": "OPEN",
    "reviewDecision": "APPROVED",
    "mergeable": "MERGEABLE",
    "additions": 10,
    "deletions": 2,
    "url": "https://github.com/org/repo/pull/1",
    "statusCheckRollup": [{"name":"test","status":"COMPLETED","conclusion":"SUCCESS"}]
  },
  {
    "number": 2,
    "title": "Other",
    "headRefName": "other",
    "headRefOid": "sha-other",
    "baseRefName": "main",
    "state": "OPEN",
    "reviewDecision": "REVIEW_REQUIRED",
    "mergeable": "UNKNOWN",
    "additions": 3,
    "deletions": 4,
    "url": "https://github.com/org/repo/pull/2",
    "statusCheckRollup": []
  }
]
JSON
`
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write gh stub: %v", err)
	}
	t.Setenv("PATH", binDir)

	got, err := BatchPRInfo(t.TempDir(), []string{"feature-a", "missing"})
	if err != nil {
		t.Fatalf("BatchPRInfo() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("BatchPRInfo() returned %d entries, want 1: %#v", len(got), got)
	}
	pr := got["feature-a"]
	if pr == nil {
		t.Fatalf("feature-a PR missing from result: %#v", got)
	}
	if pr.Number != 1 || pr.Title != "Feature A" || pr.CIStatus() != "pass" {
		t.Fatalf("unexpected feature-a PR: %+v", pr)
	}
	if got["other"] != nil {
		t.Fatalf("unrequested branch should be filtered out: %#v", got)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read gh log: %v", err)
	}
	if string(logData) != "pr list --json number,title,headRefName,headRefOid,baseRefName,state,mergedAt,reviewDecision,mergeable,additions,deletions,url,statusCheckRollup --limit 100 --state all\n" {
		t.Fatalf("unexpected gh invocation:\n%s", logData)
	}
}
