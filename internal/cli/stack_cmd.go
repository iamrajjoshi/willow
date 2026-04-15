package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errs"
	"github.com/iamrajjoshi/willow/internal/gh"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/ui"
	"github.com/urfave/cli/v3"
)

func stackCmd() *cli.Command {
	return &cli.Command{
		Name:  "stack",
		Usage: "Manage stacked branches",
		Commands: []*cli.Command{
			stackStatusCmd(),
		},
	}
}

func stackStatusCmd() *cli.Command {
	return &cli.Command{
		Name:    "status",
		Aliases: []string{"s"},
		Usage:   "Show CI and PR status for branches in a stack",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target a willow-managed repo by name",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			g := flags.NewGit()
			u := flags.NewUI()

			var bareDir string
			var err error
			if repoFlag := cmd.String("repo"); repoFlag != "" {
				bareDir, err = config.ResolveRepo(repoFlag)
				if err != nil {
					return err
				}
			} else {
				bareDir, err = requireWillowRepo(g)
				if err != nil {
					return err
				}
			}

			st := stack.Load(bareDir)
			if st.IsEmpty() {
				return errs.Userf("no stacked branches found")
			}

			branches := st.TopoSort()

			branchSet := make(map[string]bool, len(branches))
			for _, b := range branches {
				branchSet[b] = true
			}
			treeLines := st.TreeLines(branchSet)

			repoName := repoNameFromDir(bareDir)
			wtDir := filepath.Join(config.WorktreesDir(), repoName)
			ghDir, err := findGHDir(wtDir)
			if err != nil {
				return errs.Userf("no worktree found to run gh in (need at least one worktree)")
			}

			prMap, err := gh.BatchPRInfo(ghDir, branches)
			if err != nil {
				return err
			}

			if cmd.Bool("json") {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(prMap)
			}

			maxBranchWidth := 0
			for _, tl := range treeLines {
				w := utf8.RuneCountInString(tl.Prefix) + utf8.RuneCountInString(tl.Branch)
				if w > maxBranchWidth {
					maxBranchWidth = w
				}
			}

			for _, tl := range treeLines {
				branchDisplay := tl.Prefix + tl.Branch
				plainWidth := utf8.RuneCountInString(branchDisplay)
				padding := maxBranchWidth - plainWidth
				if padding < 0 {
					padding = 0
				}

				pr := prMap[tl.Branch]
				var annotation string
				if pr == nil {
					annotation = u.Dim("(no PR)")
				} else {
					annotation = formatPRAnnotation(u, pr)
				}

				line := fmt.Sprintf("  %s%s  %s", branchDisplay, strings.Repeat(" ", padding), annotation)
				u.Info(line)
			}

			return nil
		},
	}
}

func formatPRAnnotation(u *ui.UI, pr *gh.PRInfo) string {
	prNum := fmt.Sprintf("#%d", pr.Number)

	var ciLabel string
	switch pr.CIStatus() {
	case "pass":
		ciLabel = u.Green("\u2713 CI")
	case "fail":
		ciLabel = u.Red("\u2717 CI")
	case "pending":
		ciLabel = u.Yellow("\u25CB CI")
	default:
		ciLabel = u.Dim("- CI")
	}

	var reviewLabel string
	switch pr.ReviewStatus {
	case "APPROVED":
		reviewLabel = u.Green("\u2713 Review")
	case "CHANGES_REQUESTED":
		reviewLabel = u.Red("\u2717 Review")
	case "REVIEW_REQUIRED":
		reviewLabel = u.Yellow("\u25CB Review")
	default:
		reviewLabel = u.Dim("- Review")
	}

	var mergeLabel string
	switch pr.Mergeable {
	case "MERGEABLE":
		mergeLabel = u.Green("MERGEABLE")
	case "CONFLICTING":
		mergeLabel = u.Red("CONFLICTING")
	default:
		mergeLabel = u.Yellow("UNKNOWN")
	}

	diffStat := fmt.Sprintf("%s %s", u.Green(fmt.Sprintf("+%d", pr.Additions)), u.Red(fmt.Sprintf("-%d", pr.Deletions)))

	return fmt.Sprintf("%s  %s  %s  %s  %s", prNum, ciLabel, reviewLabel, mergeLabel, diffStat)
}

// findGHDir finds any existing directory under the worktrees dir to use as
// a working directory for gh commands.
func findGHDir(wtDir string) (string, error) {
	entries, err := os.ReadDir(wtDir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() {
			return filepath.Join(wtDir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no directories in %s", wtDir)
}
