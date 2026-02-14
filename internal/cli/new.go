package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/urfave/cli/v3"
)

func repoNameFromDir(bareDir string) string {
	return strings.TrimSuffix(filepath.Base(bareDir), ".git")
}

func newCmd() *cli.Command {
	return &cli.Command{
		Name:    "new",
		Aliases: []string{"n"},
		Usage:   "Create a new worktree",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "branch",
				UsageText: "<branch>",
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "base",
				Aliases: []string{"b"},
				Usage:   "Base branch to fork from",
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
			flags := parseFlags(cmd)
			g := flags.NewGit()
			u := flags.NewUI()
			cdOnly := cmd.Bool("cd")

			branch := cmd.StringArg("branch")
			if branch == "" {
				return fmt.Errorf("branch name is required\n\nUsage: ww new <branch> [flags]")
			}

			bareDir, err := g.BareRepoDir()
			if err != nil {
				return err
			}

			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			repoName := repoNameFromDir(bareDir)

			// Resolve base branch
			baseBranch := cmd.String("base")
			if baseBranch == "" {
				baseBranch, err = repoGit.DefaultBranch()
				if err != nil {
					return fmt.Errorf("failed to detect default branch (use --base to specify): %w", err)
				}
			}

			// Fetch latest from remote
			if !cmd.Bool("no-fetch") {
				if !cdOnly {
					u.Info(fmt.Sprintf("Fetching %s from origin...", u.Bold(baseBranch)))
				}
				if _, err := repoGit.Run("fetch", "origin", baseBranch); err != nil {
					return fmt.Errorf("failed to fetch origin/%s: %w", baseBranch, err)
				}
			}

			dirName := strings.ReplaceAll(branch, "/", "")
			wtPath := filepath.Join(config.WorktreesDir(), repoName, dirName)

			if cmd.Bool("existing") {
				if !cdOnly {
					u.Info(fmt.Sprintf("Creating worktree for existing branch %s...", u.Bold(branch)))
				}
				if _, err := repoGit.Run("worktree", "add", wtPath, branch); err != nil {
					return fmt.Errorf("failed to create worktree: %w", err)
				}
			} else {
				if !cdOnly {
					u.Info(fmt.Sprintf("Creating worktree %s from %s...", u.Bold(branch), u.Bold("origin/"+baseBranch)))
				}
				if _, err := repoGit.Run("worktree", "add", wtPath, "-b", branch, "origin/"+baseBranch); err != nil {
					return fmt.Errorf("failed to create worktree: %w", err)
				}
			}

			// Configure push.autoSetupRemote so `git push` works without --set-upstream
			wtGit := &git.Git{Dir: wtPath, Verbose: g.Verbose}
			if _, err := wtGit.Run("config", "--local", "push.autoSetupRemote", "true"); err != nil {
				return fmt.Errorf("failed to configure push.autoSetupRemote: %w", err)
			}

			if cdOnly {
				fmt.Println(wtPath)
				return nil
			}

			u.Success(fmt.Sprintf("Created worktree %s", u.Bold(branch)))
			u.Info(fmt.Sprintf("  path:   %s", u.Dim(wtPath)))
			u.Info(fmt.Sprintf("  base:   %s", u.Dim("origin/"+baseBranch)))
			return nil
		},
	}
}
