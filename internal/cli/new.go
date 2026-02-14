package cli

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

func newCmd() *cli.Command {
	return &cli.Command{
		Name:    "new",
		Aliases: []string{"n"},
		Usage:   "Create a new worktree",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "base",
				Aliases: []string{"b"},
				Usage:   "Base branch to fork from",
			},
			&cli.StringFlag{
				Name:  "name",
				Usage: "Human-friendly workspace name",
			},
			&cli.BoolFlag{
				Name:    "existing",
				Aliases: []string{"e"},
				Usage:   "Use an existing local/remote branch",
			},
			&cli.BoolFlag{
				Name:  "no-fetch",
				Usage: "Skip fetching latest from remote",
			},
			&cli.BoolFlag{
				Name:  "cd",
				Usage: "Print only the worktree path to stdout",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			branch := cmd.Args().First()
			if branch == "" {
				branch = "(auto)"
			}
			fmt.Printf("[stub] new worktree branch=%s base=%s\n", branch, cmd.String("base"))
			return nil
		},
	}
}
