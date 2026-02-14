package cli

import (
	"context"
	"fmt"

	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/urfave/cli/v3"
)

func pruneCmd() *cli.Command {
	return &cli.Command{
		Name:  "prune",
		Usage: "Clean up stale worktrees",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would be pruned without doing it",
			},
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Skip confirmation",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			g := flags.NewGit()
			u := flags.NewUI()

			bareDir, err := requireWillowRepo(g)
			if err != nil {
				return err
			}

			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}

			// Preview what would be pruned
			preview, err := repoGit.Run("worktree", "prune", "--dry-run", "-v")
			if err != nil {
				return fmt.Errorf("failed to check stale worktrees: %w", err)
			}

			if preview == "" {
				u.Info("Nothing to prune.")
				return nil
			}

			u.Info(preview)

			if cmd.Bool("dry-run") {
				return nil
			}

			if !cmd.Bool("yes") {
				if !confirm("Prune stale worktrees?") {
					u.Info("Aborted.")
					return nil
				}
			}

			if _, err := repoGit.Run("worktree", "prune", "-v"); err != nil {
				return fmt.Errorf("failed to prune worktrees: %w", err)
			}

			u.Success("Pruned stale worktrees")
			return nil
		},
	}
}
