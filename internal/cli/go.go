package cli

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

func goCmd() *cli.Command {
	return &cli.Command{
		Name:    "go",
		Aliases: []string{"g"},
		Usage:   "Print worktree path / interactive picker",
		Action: func(_ context.Context, cmd *cli.Command) error {
			target := cmd.Args().First()
			if target == "" {
				fmt.Println("[stub] interactive worktree picker")
				return nil
			}
			fmt.Printf("[stub] go to worktree=%s\n", target)
			return nil
		},
	}
}
