package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func lsCmd() *cli.Command {
	return &cli.Command{
		Name:    "ls",
		Aliases: []string{"l"},
		Usage:   "List all worktrees",
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

			bareDir, err := g.BareRepoDir()
			if err != nil {
				return err
			}

			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			worktrees, err := worktree.List(repoGit)
			if err != nil {
				return fmt.Errorf("failed to list worktrees: %w", err)
			}

			// Filter out the bare repo entry
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
		},
	}
}

func printTable(flags Flags, worktrees []worktree.Worktree) {
	u := flags.NewUI()

	if len(worktrees) == 0 {
		u.Info("No worktrees found.")
		return
	}

	// Compute column widths
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
