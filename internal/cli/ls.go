package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
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
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			g := flags.NewGit()

			// If an explicit repo name was given, resolve it and list its worktrees
			if repoArg := cmd.StringArg("repo"); repoArg != "" {
				bareDir, err := config.ResolveRepo(repoArg)
				if err != nil {
					return err
				}
				return listWorktrees(flags, cmd, &git.Git{Dir: bareDir, Verbose: g.Verbose})
			}

			// Try to detect the current repo
			bareDir, err := g.BareRepoDir()
			if err == nil && config.IsWillowRepo(bareDir) {
				return listWorktrees(flags, cmd, &git.Git{Dir: bareDir, Verbose: g.Verbose})
			}

			// Outside a willow repo — list all repos
			return printRepoList(flags)
		},
	}
}

func listWorktrees(flags Flags, cmd *cli.Command, repoGit *git.Git) error {
	worktrees, err := worktree.List(repoGit)
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	var filtered []worktree.Worktree
	for _, wt := range worktrees {
		if !wt.IsBare {
			filtered = append(filtered, wt)
		}
	}

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

	printTable(flags, filtered)
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

	header := fmt.Sprintf("  %-*s  %s", nameW, "REPO", "WORKTREES")
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
		for _, wt := range wts {
			if !wt.IsBare {
				count++
			}
		}
		line := fmt.Sprintf("  %-*s  %d", nameW, r, count)
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

func printTable(flags Flags, worktrees []worktree.Worktree) {
	u := flags.NewUI()

	if len(worktrees) == 0 {
		u.Info("No worktrees found.")
		return
	}

	branchW := len("BRANCH")
	pathW := len("PATH")
	for _, wt := range worktrees {
		if len(wt.Branch) > branchW {
			branchW = len(wt.Branch)
		}
		if len(wt.Path) > pathW {
			pathW = len(wt.Path)
		}
	}

	header := fmt.Sprintf("  %-*s  %-*s  %s", branchW, "BRANCH", pathW, "PATH", "AGE")
	u.Info(u.Bold(header))

	for _, wt := range worktrees {
		age := worktreeAge(wt.Path)
		line := fmt.Sprintf("  %-*s  %-*s  %s", branchW, wt.Branch, pathW, u.Dim(wt.Path), u.Dim(age))
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
