package cli

import (
	"context"
	"fmt"

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
			fmt.Println("[stub] listing worktrees")
			fmt.Println(" BRANCH                STATUS    PATH                                         AGE")
			fmt.Println(" main                  clean     ~/.willow/worktrees/myrepo/main               3d")
			return nil
		},
	}
}
