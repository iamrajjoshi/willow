package gh

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/iamrajjoshi/willow/internal/errs"
)

type checkRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`     // COMPLETED, IN_PROGRESS, QUEUED
	Conclusion string `json:"conclusion"` // SUCCESS, FAILURE, NEUTRAL, SKIPPED, etc.
}

type ghPR struct {
	Number       int        `json:"number"`
	Title        string     `json:"title"`
	Branch       string     `json:"headRefName"`
	State        string     `json:"state"`
	ReviewStatus string     `json:"reviewDecision"`
	Mergeable    string     `json:"mergeable"`
	Additions    int        `json:"additions"`
	Deletions    int        `json:"deletions"`
	URL          string     `json:"url"`
	Checks       []checkRun `json:"statusCheckRollup"`
}

type PRInfo struct {
	Number       int    `json:"number"`
	Title        string `json:"title"`
	Branch       string `json:"headRefName"`
	State        string `json:"state"`         // OPEN, MERGED, CLOSED
	ReviewStatus string `json:"reviewDecision"` // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, ""
	Mergeable    string `json:"mergeable"`      // MERGEABLE, CONFLICTING, UNKNOWN
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	URL          string `json:"url"`
	Checks       []checkRun
}

// CIStatus returns the aggregate CI status: "pass", "fail", "pending", or "none".
func (p *PRInfo) CIStatus() string {
	if len(p.Checks) == 0 {
		return "none"
	}
	for _, c := range p.Checks {
		if c.Conclusion == "FAILURE" {
			return "fail"
		}
	}
	for _, c := range p.Checks {
		if c.Status != "COMPLETED" {
			return "pending"
		}
	}
	return "pass"
}

// BatchPRInfo fetches PR info for multiple branches in a single gh call.
func BatchPRInfo(dir string, branches []string) (map[string]*PRInfo, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, errs.Userf("gh CLI required for stack status (install: https://cli.github.com)")
	}

	cmd := exec.Command("gh", "pr", "list",
		"--json", "number,title,headRefName,state,reviewDecision,mergeable,additions,deletions,url,statusCheckRollup",
		"--limit", "100",
		"--state", "all",
	)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr list failed: %w", err)
	}

	var prs []ghPR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("failed to parse gh output: %w", err)
	}

	branchSet := make(map[string]bool, len(branches))
	for _, b := range branches {
		branchSet[b] = true
	}

	result := make(map[string]*PRInfo)
	for _, pr := range prs {
		if branchSet[pr.Branch] {
			result[pr.Branch] = &PRInfo{
				Number:       pr.Number,
				Title:        pr.Title,
				Branch:       pr.Branch,
				State:        pr.State,
				ReviewStatus: pr.ReviewStatus,
				Mergeable:    pr.Mergeable,
				Additions:    pr.Additions,
				Deletions:    pr.Deletions,
				URL:          pr.URL,
				Checks:       pr.Checks,
			}
		}
	}
	return result, nil
}
