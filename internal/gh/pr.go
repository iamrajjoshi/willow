package gh

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

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
const openPRLookupLimit = 20

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

func prLookupArgs(branch string) []string {
	return []string{
		"pr", "list",
		"--head", branch,
		"--state", "open",
		"--json", prJSONFields,
		"--limit", fmt.Sprintf("%d", openPRLookupLimit),
	}
}

func prCreateArgs(base, head string, draft bool) []string {
	args := []string{"pr", "create", "--fill", "--base", base, "--head", head}
	if draft {
		args = append(args, "--draft")
	}
	return args
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

func selectMatchingPR(prs []*PRInfo, headOID string) *PRInfo {
	if len(prs) == 0 {
		return nil
	}
	if headOID == "" {
		if len(prs) == 1 {
			return prs[0]
		}
		return nil
	}

	for _, pr := range prs {
		if pr.HeadRefOID == headOID {
			return pr
		}
	}
	return nil
}

func FindOpenPR(dir, branch, headOID string) (*PRInfo, error) {
	if err := EnsureCLI("PR creation"); err != nil {
		return nil, err
	}

	cmd := exec.Command("gh", prLookupArgs(branch)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GH_PROMPT_DISABLED=1",
		"GH_NO_UPDATE_NOTIFIER=1",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return nil, fmt.Errorf("gh pr list failed: %s", msg)
		}
		return nil, fmt.Errorf("gh pr list failed: %w", err)
	}

	prs, err := parsePRListOutput(out)
	if err != nil {
		return nil, err
	}
	return selectMatchingPR(prs, headOID), nil
}

func CreatePR(dir, base, head string, draft bool) (string, error) {
	if err := EnsureCLI("PR creation"); err != nil {
		return "", err
	}

	cmd := exec.Command("gh", prCreateArgs(base, head, draft)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GH_PROMPT_DISABLED=1",
		"GH_NO_UPDATE_NOTIFIER=1",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return "", fmt.Errorf("gh pr create failed: %s", msg)
		}
		return "", fmt.Errorf("gh pr create failed: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
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
