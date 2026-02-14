package cli

import (
	"fmt"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/ui"
	"github.com/urfave/cli/v3"
)

var errNotWillowRepo = fmt.Errorf("not inside a willow-managed repo\n\nRun this command from a worktree under ~/.willow, or use 'ww ls' to see your repos.")

func requireWillowRepo(g *git.Git) (string, error) {
	bareDir, err := g.BareRepoDir()
	if err != nil {
		return "", errNotWillowRepo
	}
	if !config.IsWillowRepo(bareDir) {
		return "", errNotWillowRepo
	}
	return bareDir, nil
}

var version = "dev"

type Flags struct {
	Verbose bool
	NoColor bool
}

func parseFlags(cmd *cli.Command) Flags {
	return Flags{
		Verbose: cmd.Root().Bool("verbose"),
		NoColor: cmd.Root().Bool("no-color"),
	}
}

func (f Flags) NewGit() *git.Git {
	return &git.Git{Verbose: f.Verbose}
}

func (f Flags) NewUI() *ui.UI {
	return &ui.UI{NoColor: f.NoColor}
}

func NewApp() *cli.Command {
	return &cli.Command{
		Name:                  "willow",
		Usage:                 "A simple, opinionated git worktree manager",
		Version:               version,
		EnableShellCompletion: true,
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
			cloneCmd(),
			newCmd(),
			lsCmd(),
			pwdCmd(),
			rmCmd(),
			runCmd(),
			pruneCmd(),
			initCmd(),
			configCmd(),
			shellInitCmd(),
		},
	}
}
