package cli

import (
	"github.com/urfave/cli/v3"
)

var version = "dev"

func NewApp() *cli.Command {
	return &cli.Command{
		Name:    "willow",
		Usage:   "A simple, opinionated git worktree manager",
		Version: version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "C",
				Usage:   "Run as if willow was started in `PATH`",
				Sources: cli.EnvVars("WILLOW_DIR"),
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "Show git commands being executed",
			},
			&cli.BoolFlag{
				Name:  "no-color",
				Usage: "Disable colored output",
			},
		},
		Commands: []*cli.Command{
			newCmd(),
			lsCmd(),
			goCmd(),
		},
	}
}
