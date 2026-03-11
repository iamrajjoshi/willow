package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/fzf"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func swCmd() *cli.Command {
	return &cli.Command{
		Name:  "sw",
		Usage: "Switch to a worktree (fzf picker)",
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			g := flags.NewGit()

			bareDir, err := requireWillowRepo(g)
			if err != nil {
				return err
			}

			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			worktrees, err := worktree.List(repoGit)
			if err != nil {
				return fmt.Errorf("failed to list worktrees: %w", err)
			}

			filtered := filterBareWorktrees(worktrees)
			if len(filtered) == 0 {
				return fmt.Errorf("no worktrees found")
			}

			repoName := repoNameFromDir(bareDir)
			selected, err := fzfPickWorktree(filtered, repoName)
			if err != nil {
				return err
			}
			if selected == "" {
				return nil
			}

			// Mark sessions as read when switching to a worktree
			wtDir := filepath.Base(selected)
			claude.MarkRead(repoName, wtDir)

			fmt.Println(selected)
			return nil
		},
	}
}

type worktreeWithStatus struct {
	wt     worktree.Worktree
	status *claude.WorktreeStatus
}

func fzfPickWorktree(worktrees []worktree.Worktree, repoName string) (string, error) {
	items := make([]worktreeWithStatus, len(worktrees))
	for i, wt := range worktrees {
		wtDir := filepath.Base(wt.Path)
		items[i] = worktreeWithStatus{
			wt:     wt,
			status: claude.ReadStatus(repoName, wtDir),
		}
	}

	// Sort: BUSY first, then WAIT, then IDLE, then OFFLINE
	sort.SliceStable(items, func(i, j int) bool {
		return statusOrder(items[i].status.Status) < statusOrder(items[j].status.Status)
	})

	branchW := 0
	statusW := 4 // len("BUSY")
	for _, item := range items {
		if len(item.wt.Branch) > branchW {
			branchW = len(item.wt.Branch)
		}
	}

	var lines []string
	for _, item := range items {
		icon := claude.StatusIcon(item.status.Status)
		label := claude.StatusLabel(item.status.Status)
		line := fmt.Sprintf("%s %-*s  %-*s  %s",
			icon,
			statusW, label,
			branchW, item.wt.Branch,
			item.wt.Path,
		)
		lines = append(lines, line)
	}

	selected, err := fzf.Run(lines,
		fzf.WithAnsi(),
		fzf.WithReverse(),
		fzf.WithHeader("Select worktree"),
	)
	if err != nil {
		return "", err
	}
	if selected == "" {
		return "", nil
	}

	return extractPathFromLine(selected), nil
}

func extractPathFromLine(line string) string {
	// The path is the last whitespace-separated field
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func statusOrder(s claude.Status) int {
	switch s {
	case claude.StatusBusy:
		return 0
	case claude.StatusWait:
		return 1
	case claude.StatusDone:
		return 2
	case claude.StatusIdle:
		return 3
	default:
		return 4
	}
}
