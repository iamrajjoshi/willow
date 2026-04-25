package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errors"
	"github.com/iamrajjoshi/willow/internal/fzf"
	"github.com/iamrajjoshi/willow/internal/gh"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/parallel"
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
					return errors.Userf("no worktrees found")
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
				return errors.Userf("no worktrees found")
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
	unread bool
	merged bool
}

func buildWorktreeLines(worktrees []worktree.Worktree, repoName string) []string {
	mergedSet := mergedBranchSetForRepo(repoName, "", worktrees)
	items := make([]worktreeWithStatus, len(worktrees))
	for i, wt := range worktrees {
		wtDir := filepath.Base(wt.Path)
		sessions := claude.ReadAllSessions(repoName, wtDir)
		ws := claude.AggregateStatus(sessions)
		items[i] = worktreeWithStatus{
			wt:     wt,
			status: ws,
			unread: ws.Status == claude.StatusDone && claude.CountUnreadIn(repoName, wtDir, sessions) > 0,
			merged: !wt.Detached && mergedSet[wt.Branch],
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].merged != items[j].merged {
			return !items[i].merged
		}
		return claude.WorktreeUrgencyOrder(items[i].status.Status, items[i].unread) <
			claude.WorktreeUrgencyOrder(items[j].status.Status, items[j].unread)
	})

	branchW := 0
	statusW := 4 // len("BUSY")
	for _, item := range items {
		display := item.wt.DisplayName()
		if len(display) > branchW {
			branchW = len(display)
		}
	}

	var lines []string
	for _, item := range items {
		icon := claude.StatusIcon(item.status.Status)
		label := claude.StatusLabel(item.status.Status)
		if item.unread {
			label += "\u25CF"
		}
		line := fmt.Sprintf("%s %-*s  %-*s  %s",
			icon,
			statusW, label,
			branchW, item.wt.DisplayName(),
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
		unread bool
		merged bool
	}

	mergedSets := make(map[string]map[string]bool)
	grouped := make(map[string][]worktree.Worktree)
	var repoOrder []repoInfo
	for _, rwt := range rwts {
		if _, ok := grouped[rwt.Repo.Name]; !ok {
			repoOrder = append(repoOrder, rwt.Repo)
		}
		grouped[rwt.Repo.Name] = append(grouped[rwt.Repo.Name], rwt.Worktree)
	}

	type mergedResult struct {
		repoName string
		merged   map[string]bool
	}
	results := parallel.Map(repoOrder, func(_ int, repo repoInfo) mergedResult {
		return mergedResult{
			repoName: repo.Name,
			merged:   mergedBranchSetForRepo(repo.Name, repo.BareDir, grouped[repo.Name]),
		}
	})
	for _, result := range results {
		mergedSets[result.repoName] = result.merged
	}

	items := make([]item, len(rwts))
	for i, rwt := range rwts {
		wtDir := filepath.Base(rwt.Worktree.Path)
		sessions := claude.ReadAllSessions(rwt.Repo.Name, wtDir)
		ws := claude.AggregateStatus(sessions)
		items[i] = item{
			rwt:    rwt,
			status: ws,
			unread: ws.Status == claude.StatusDone && claude.CountUnreadIn(rwt.Repo.Name, wtDir, sessions) > 0,
			merged: !rwt.Worktree.Detached && mergedSets[rwt.Repo.Name][rwt.Worktree.Branch],
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].merged != items[j].merged {
			return !items[i].merged
		}
		return claude.WorktreeUrgencyOrder(items[i].status.Status, items[i].unread) <
			claude.WorktreeUrgencyOrder(items[j].status.Status, items[j].unread)
	})

	nameW := 0
	statusW := 4
	for _, it := range items {
		display := it.rwt.Repo.Name + "/" + it.rwt.Worktree.DisplayName()
		if len(display) > nameW {
			nameW = len(display)
		}
	}

	var lines []string
	for _, it := range items {
		icon := claude.StatusIcon(it.status.Status)
		label := claude.StatusLabel(it.status.Status)
		if it.unread {
			label += "\u25CF"
		}
		display := it.rwt.Repo.Name + "/" + it.rwt.Worktree.DisplayName()
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

func mergedBranchSetForRepo(repoName, bareDir string, worktrees []worktree.Worktree) map[string]bool {
	if bareDir == "" {
		resolved, err := config.ResolveRepo(repoName)
		if err != nil {
			return map[string]bool{}
		}
		bareDir = resolved
	}

	branches := make([]string, 0, len(worktrees))
	branchHeads := make(map[string]string, len(worktrees))
	repoDir := ""
	for _, wt := range worktrees {
		if wt.Branch != "" && !wt.Detached {
			if repoDir == "" {
				repoDir = wt.Path
			}
			branches = append(branches, wt.Branch)
			branchHeads[wt.Branch] = wt.Head
		}
	}
	if len(branches) == 0 {
		return map[string]bool{}
	}

	repoGit := &git.Git{Dir: bareDir}
	cfg := config.Load(bareDir)
	baseBranch := repoGit.ResolveBaseBranch(cfg.BaseBranch)
	mergedSet := repoGit.MergedBranchSet(baseBranch, branches)
	for branch := range gh.MergedWorktreeSet(repoDir, baseBranch, branchHeads) {
		mergedSet[branch] = true
	}
	return mergedSet
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
