package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func lsCmd() *cli.Command {
	return &cli.Command{
		Name:    "ls",
		Aliases: []string{"l"},
		Usage:   "List worktrees (or repos when outside a willow repo)",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "repo",
				UsageText: "[repo]",
			},
		},
		ShellComplete: completeRepos,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
			&cli.BoolFlag{
				Name:  "path-only",
				Usage: "Print only worktree paths",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.ls")()
			flags := parseFlags(cmd)
			g := flags.NewGit()

			if repoArg := cmd.StringArg("repo"); repoArg != "" {
				bareDir, err := config.ResolveRepo(repoArg)
				if err != nil {
					return err
				}
				return listWorktrees(ctx, flags, cmd, &git.Git{Dir: bareDir, Verbose: g.Verbose})
			}

			bareDir, err := g.BareRepoDir()
			if err == nil && config.IsWillowRepo(bareDir) {
				return listWorktrees(ctx, flags, cmd, &git.Git{Dir: bareDir, Verbose: g.Verbose})
			}
			if bareDir, ok := resolveRepoFromCwd(); ok {
				return listWorktrees(ctx, flags, cmd, &git.Git{Dir: bareDir, Verbose: g.Verbose})
			}

			return printRepoList(flags)
		},
	}
}

func listWorktrees(ctx context.Context, flags Flags, cmd *cli.Command, repoGit *git.Git) error {
	done := trace.Span(ctx, "worktree.List")
	worktrees, err := worktree.List(repoGit)
	done()
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	filtered := filterBareWorktrees(worktrees)

	if cmd.Bool("path-only") {
		for _, wt := range filtered {
			fmt.Println(wt.Path)
		}
		return nil
	}

	if cmd.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(filtered)
	}

	repoName := repoNameFromDir(repoGit.Dir)
	printTable(ctx, flags, filtered, repoName, repoGit)
	return nil
}

func printRepoList(flags Flags) error {
	u := flags.NewUI()
	repos, err := config.ListRepos()
	if err != nil {
		return fmt.Errorf("failed to list repos: %w", err)
	}

	if len(repos) == 0 {
		u.Info("No willow-managed repos. Use 'ww clone <url>' to get started.")
		return nil
	}

	nameW := len("REPO")
	for _, r := range repos {
		if len(r) > nameW {
			nameW = len(r)
		}
	}

	header := fmt.Sprintf("  %-*s  %-9s  %-6s  %s", nameW, "REPO", "WORKTREES", "ACTIVE", "UNREAD")
	u.Info(u.Bold(header))

	for _, r := range repos {
		bareDir, err := config.ResolveRepo(r)
		if err != nil {
			continue
		}
		repoGit := &git.Git{Dir: bareDir}
		wts, err := worktree.List(repoGit)
		if err != nil {
			continue
		}
		count := 0
		activeCount := 0
		unreadCount := 0
		for _, wt := range wts {
			if wt.IsBare {
				continue
			}
			count++
			wtDir := filepath.Base(wt.Path)
			ws := claude.ReadStatus(r, wtDir)
			if claude.IsActive(ws.Status) {
				activeCount++
			}
			if claude.IsUnread(r, wtDir) {
				unreadCount++
			}
		}
		line := fmt.Sprintf("  %-*s  %-9d  %-6d  %d", nameW, r, count, activeCount, unreadCount)
		u.Info(line)
	}
	return nil
}

func completeRepos(ctx context.Context, cmd *cli.Command) {
	if args := cmd.Args().Slice(); len(args) > 0 && strings.HasPrefix(args[len(args)-1], "-") {
		cli.DefaultCompleteWithFlags(ctx, cmd)
		return
	}

	repos, err := config.ListRepos()
	if err != nil {
		return
	}
	w := cmd.Root().Writer
	for _, r := range repos {
		fmt.Fprintln(w, r)
	}
}

type lsRow struct {
	branch      string
	prefix      string // tree-drawing prefix
	merged      bool
	wt          worktree.Worktree
}

func printTable(ctx context.Context, flags Flags, worktrees []worktree.Worktree, repoName string, repoGit *git.Git) {
	u := flags.NewUI()

	if len(worktrees) == 0 {
		u.Info("No worktrees found.")
		return
	}

	cfg := config.Load(repoGit.Dir)

	st := stack.Load(repoGit.Dir)
	branchSet := make(map[string]bool)
	wtMap := make(map[string]worktree.Worktree)
	branches := make([]string, 0, len(worktrees))
	for _, wt := range worktrees {
		branchSet[wt.Branch] = true
		wtMap[wt.Branch] = wt
		if wt.Branch != "" {
			branches = append(branches, wt.Branch)
		}
	}
	done := trace.Span(ctx, "MergedBranchSet")
	mergedSet := repoGit.MergedBranchSet(repoGit.ResolveBaseBranch(cfg.BaseBranch), branches)
	done()

	var rows []lsRow
	stackedBranches := make(map[string]bool)

	if !st.IsEmpty() {
		treeLines := st.TreeLines(branchSet)
		for _, tl := range treeLines {
			if wt, ok := wtMap[tl.Branch]; ok {
				stackedBranches[tl.Branch] = true
				rows = append(rows, lsRow{
					branch: tl.Branch,
					prefix: tl.Prefix,
					merged: mergedSet[tl.Branch],
					wt:     wt,
				})
			}
		}
	}

	for _, wt := range worktrees {
		if !stackedBranches[wt.Branch] {
			rows = append(rows, lsRow{
				branch: wt.Branch,
				merged: mergedSet[wt.Branch],
				wt:     wt,
			})
		}
	}

	branchW := len("BRANCH")
	statusW := len("STATUS")
	pathW := len("PATH")
	ageW := len("AGE")
	for _, row := range rows {
		display := row.prefix + row.branch
		if row.merged {
			display += " [merged]"
		}
		if utf8.RuneCountInString(display) > branchW {
			branchW = utf8.RuneCountInString(display)
		}
		if len(row.wt.Path) > pathW {
			pathW = len(row.wt.Path)
		}
		age := worktreeAge(row.wt.Path)
		if len(age) > ageW {
			ageW = len(age)
		}
	}

	header := fmt.Sprintf("  %-*s  %-*s  %-*s  %*s", branchW, "BRANCH", statusW, "STATUS", pathW, "PATH", ageW, "AGE")
	u.Info(u.Bold(header))

	for _, row := range rows {
		age := worktreeAge(row.wt.Path)
		wtDir := filepath.Base(row.wt.Path)
		ws := claude.ReadStatus(repoName, wtDir)
		statusLabel := claude.StatusLabel(ws.Status)
		if ws.Status == claude.StatusDone && claude.IsUnread(repoName, wtDir) {
			statusLabel += "\u25CF" // ●
		}
		branchPlain := row.prefix + row.branch
		branchDisplay := branchPlain
		if row.merged {
			branchPlain += " [merged]"
			branchDisplay += " " + u.Dim("[merged]")
		}
		// Pad based on plain text width to avoid ANSI code miscount
		padding := branchW - utf8.RuneCountInString(branchPlain)
		if padding < 0 {
			padding = 0
		}
		branchCol := branchDisplay + strings.Repeat(" ", padding)
		pathPadded := fmt.Sprintf("%-*s", pathW, row.wt.Path)
		agePadded := fmt.Sprintf("%*s", ageW, age)
		line := fmt.Sprintf("  %s  %-*s  %s  %s", branchCol, statusW, statusLabel, u.Dim(pathPadded), u.Dim(agePadded))
		u.Info(line)
	}
}

func worktreeAge(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "?"
	}
	return formatAge(time.Since(info.ModTime()))
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	}
}
