package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/ui"
	"github.com/urfave/cli/v3"
)

func repoNameFromDir(bareDir string) string {
	return strings.TrimSuffix(filepath.Base(bareDir), ".git")
}

func runHooks(commands []string, dir string, u *ui.UI) error {
	for _, c := range commands {
		u.Info(fmt.Sprintf("  → %s", c))
		sh := exec.Command("sh", "-c", c)
		sh.Dir = dir
		sh.Stdout = os.Stdout
		sh.Stderr = os.Stderr
		if err := sh.Run(); err != nil {
			return fmt.Errorf("hook failed: %s: %w", c, err)
		}
	}
	return nil
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

			worktreeRoot, _ := g.WorktreeRoot()
			cfg := config.Load(bareDir, worktreeRoot)

			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			repoName := repoNameFromDir(bareDir)

			// Apply branch prefix from config
			if cfg.BranchPrefix != "" && !strings.HasPrefix(branch, cfg.BranchPrefix+"/") {
				branch = cfg.BranchPrefix + "/" + branch
			}

			// Resolve base branch: flag → config → auto-detect
			baseBranch := cmd.String("base")
			if baseBranch == "" {
				baseBranch = cfg.BaseBranch
			}
			if baseBranch == "" {
				baseBranch, err = repoGit.DefaultBranch()
				if err != nil {
					return fmt.Errorf("failed to detect default branch (use --base to specify): %w", err)
				}
			}

			// Fetch latest from remote (config default, --no-fetch overrides)
			shouldFetch := *cfg.Defaults.Fetch && !cmd.Bool("no-fetch")
			if shouldFetch {
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

			if *cfg.Defaults.AutoSetupRemote {
				wtGit := &git.Git{Dir: wtPath, Verbose: g.Verbose}
				if _, err := wtGit.Run("config", "--local", "push.autoSetupRemote", "true"); err != nil {
					return fmt.Errorf("failed to configure push.autoSetupRemote: %w", err)
				}
			}

			// Run setup hooks
			if len(cfg.Setup) > 0 && !cdOnly {
				u.Info("Running setup hooks...")
				if err := runHooks(cfg.Setup, wtPath, u); err != nil {
					return err
				}
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
