package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func runCmd() *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "Run a command in a worktree",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "Run in all worktrees",
			},
		},
		SkipFlagParsing: true,
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			g := flags.NewGit()
			u := flags.NewUI()

			args := cmd.Args().Slice()
			if len(args) == 0 {
				return fmt.Errorf("usage: ww run <branch-or-name> -- <command...>\n       ww run --all -- <command...>")
			}

			bareDir, err := g.BareRepoDir()
			if err != nil {
				return err
			}

			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			worktrees, err := worktree.List(repoGit)
			if err != nil {
				return fmt.Errorf("failed to list worktrees: %w", err)
			}

			// Filter out bare repo
			var filtered []worktree.Worktree
			for _, wt := range worktrees {
				if !wt.IsBare {
					filtered = append(filtered, wt)
				}
			}

			// Parse: either --all -- <cmd> or <target> -- <cmd>
			runAll := false
			var target string
			var cmdArgs []string

			for i, arg := range args {
				if arg == "--" {
					cmdArgs = args[i+1:]
					break
				}
				if arg == "--all" {
					runAll = true
				} else {
					target = arg
				}
			}

			if len(cmdArgs) == 0 {
				return fmt.Errorf("no command specified after --\n\nUsage: ww run <branch-or-name> -- <command...>")
			}

			if runAll {
				for _, wt := range filtered {
					u.Info(fmt.Sprintf("==> %s", u.Bold(wt.Branch)))
					if err := execIn(wt.Path, cmdArgs); err != nil {
						u.Warn(fmt.Sprintf("command failed in %s: %v", wt.Branch, err))
					}
				}
				return nil
			}

			if target == "" {
				return fmt.Errorf("branch or worktree name is required\n\nUsage: ww run <branch-or-name> -- <command...>")
			}

			wt, err := findWorktree(filtered, target)
			if err != nil {
				return err
			}

			return execIn(wt.Path, cmdArgs)
		},
	}
}

func execIn(dir string, args []string) error {
	c := exec.Command(args[0], args[1:]...)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}
