package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/errs"
	"github.com/iamrajjoshi/willow/internal/fzf"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func swCmd() *cli.Command {
	return &cli.Command{
		Name:  "sw",
		Usage: "Switch to a worktree (fzf picker)",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "name",
				UsageText: "[name]",
			},
		},
		ShellComplete: completeWorktreesWithFlag,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target a willow-managed repo by name",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.sw")()
			flags := parseFlags(cmd)
			g := flags.NewGit()

			repos, err := resolveRepos(g, cmd.String("repo"))
			if err != nil {
				return err
			}

			multiRepo := len(repos) > 1
			name := cmd.StringArg("name")

			if name != "" {
				allWts := collectAllWorktrees(repos, g.Verbose)
				rwt, err := findCrossRepoWorktree(allWts, name)
				if err != nil {
					return err
				}
				wtDir := filepath.Base(rwt.Worktree.Path)
				claude.MarkRead(rwt.Repo.Name, wtDir)
				fmt.Println(rwt.Worktree.Path)
				return nil
			}

			if multiRepo {
				allWts := collectAllWorktrees(repos, g.Verbose)
				if len(allWts) == 0 {
					return errs.Userf("no worktrees found")
				}

				lines := buildCrossRepoWorktreeLines(allWts)
				selected, err := fzf.Run(lines,
					fzf.WithAnsi(),
					fzf.WithReverse(),
					fzf.WithHeader("Select worktree"),
				)
				if err != nil {
					return err
				}
				if selected == "" {
					return nil
				}

				path := extractPathFromLine(selected)
				if rwt := repoWorktreeByPath(allWts, path); rwt != nil {
					claude.MarkRead(rwt.Repo.Name, filepath.Base(path))
				}
				fmt.Println(path)
				return nil
			}

			bareDir := repos[0].BareDir
			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			worktrees, err := worktree.List(repoGit)
			if err != nil {
				return fmt.Errorf("failed to list worktrees: %w", err)
			}

			filtered := filterBareWorktrees(worktrees)
			if len(filtered) == 0 {
				return errs.Userf("no worktrees found")
			}

			repoName := repos[0].Name
			selected, err := fzfPickWorktree(filtered, repoName)
			if err != nil {
				return err
			}
			if selected == "" {
				return nil
			}

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

func buildWorktreeLines(worktrees []worktree.Worktree, repoName string) []string {
	items := make([]worktreeWithStatus, len(worktrees))
	for i, wt := range worktrees {
		wtDir := filepath.Base(wt.Path)
		items[i] = worktreeWithStatus{
			wt:     wt,
			status: claude.ReadStatus(repoName, wtDir),
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		return claude.StatusOrder(items[i].status.Status) < claude.StatusOrder(items[j].status.Status)
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
	return lines
}

func buildCrossRepoWorktreeLines(rwts []repoWorktree) []string {
	type item struct {
		rwt    repoWorktree
		status *claude.WorktreeStatus
	}
	items := make([]item, len(rwts))
	for i, rwt := range rwts {
		wtDir := filepath.Base(rwt.Worktree.Path)
		items[i] = item{
			rwt:    rwt,
			status: claude.ReadStatus(rwt.Repo.Name, wtDir),
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		return claude.StatusOrder(items[i].status.Status) < claude.StatusOrder(items[j].status.Status)
	})

	nameW := 0
	statusW := 4
	for _, it := range items {
		display := it.rwt.Repo.Name + "/" + it.rwt.Worktree.Branch
		if len(display) > nameW {
			nameW = len(display)
		}
	}

	var lines []string
	for _, it := range items {
		icon := claude.StatusIcon(it.status.Status)
		label := claude.StatusLabel(it.status.Status)
		display := it.rwt.Repo.Name + "/" + it.rwt.Worktree.Branch
		line := fmt.Sprintf("%s %-*s  %-*s  %s",
			icon,
			statusW, label,
			nameW, display,
			it.rwt.Worktree.Path,
		)
		lines = append(lines, line)
	}
	return lines
}

func fzfPickWorktree(worktrees []worktree.Worktree, repoName string) (string, error) {
	lines := buildWorktreeLines(worktrees, repoName)

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

func fzfPickWorktrees(worktrees []worktree.Worktree, repoName string) ([]string, error) {
	lines := buildWorktreeLines(worktrees, repoName)

	selected, err := fzf.RunMulti(lines,
		fzf.WithAnsi(),
		fzf.WithReverse(),
		fzf.WithHeader("Select worktrees (TAB to multi-select)"),
	)
	if err != nil {
		return nil, err
	}
	if selected == nil {
		return nil, nil
	}

	paths := make([]string, len(selected))
	for i, line := range selected {
		paths[i] = extractPathFromLine(line)
	}
	return paths, nil
}

func extractPathFromLine(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

