package gh

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/iamrajjoshi/willow/internal/errors"
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
	HeadRefOID   string     `json:"headRefOid"`
	BaseRefName  string     `json:"baseRefName"`
	State        string     `json:"state"`
	MergedAt     string     `json:"mergedAt"`
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
	HeadRefOID   string `json:"headRefOid"`
	BaseRefName  string `json:"baseRefName"`
	State        string `json:"state"` // OPEN, MERGED, CLOSED
	MergedAt     string `json:"mergedAt"`
	ReviewStatus string `json:"reviewDecision"` // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, ""
	Mergeable    string `json:"mergeable"`      // MERGEABLE, CONFLICTING, UNKNOWN
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	URL          string `json:"url"`
	Checks       []checkRun
}

const prJSONFields = "number,title,headRefName,headRefOid,baseRefName,state,mergedAt,reviewDecision,mergeable,additions,deletions,url,statusCheckRollup"

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

func EnsureCLI(feature string) error {
	if _, err := exec.LookPath("gh"); err != nil {
		if feature != "" {
			return errors.Userf("gh CLI required for %s (install: https://cli.github.com)", feature)
		}
		return errors.Userf("'gh' CLI not found — install it from https://cli.github.com")
	}
	return nil
}

func parsePRListOutput(out []byte) ([]*PRInfo, error) {
	var prs []ghPR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("failed to parse gh output: %w", err)
	}

	infos := make([]*PRInfo, 0, len(prs))
	for _, pr := range prs {
		infos = append(infos, &PRInfo{
			Number:       pr.Number,
			Title:        pr.Title,
			Branch:       pr.Branch,
			HeadRefOID:   pr.HeadRefOID,
			BaseRefName:  pr.BaseRefName,
			State:        pr.State,
			MergedAt:     pr.MergedAt,
			ReviewStatus: pr.ReviewStatus,
			Mergeable:    pr.Mergeable,
			Additions:    pr.Additions,
			Deletions:    pr.Deletions,
			URL:          pr.URL,
			Checks:       pr.Checks,
		})
	}
	return infos, nil
}

// BatchPRInfo fetches PR info for multiple branches in a single gh call.
func BatchPRInfo(dir string, branches []string) (map[string]*PRInfo, error) {
	if err := EnsureCLI("stack status"); err != nil {
		return nil, err
	}

	cmd := exec.Command("gh", "pr", "list",
		"--json", prJSONFields,
		"--limit", "100",
		"--state", "all",
	)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr list failed: %w", err)
	}

	prs, err := parsePRListOutput(out)
	if err != nil {
		return nil, err
	}

	branchSet := make(map[string]bool, len(branches))
	for _, b := range branches {
		branchSet[b] = true
	}

	result := make(map[string]*PRInfo)
	for _, pr := range prs {
		if branchSet[pr.Branch] {
			result[pr.Branch] = pr
		}
	}
	return result, nil
}
