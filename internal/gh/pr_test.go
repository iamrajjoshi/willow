package gh

import (
	"encoding/json"
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
