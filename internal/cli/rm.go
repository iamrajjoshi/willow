package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func rmCmd() *cli.Command {
	return &cli.Command{
		Name:  "rm",
		Usage: "Remove a worktree and its branch",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "branch",
				UsageText: "[branch]",
			},
		},
		ShellComplete: completeWorktrees,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Skip safety checks",
			},
			&cli.BoolFlag{
				Name:  "keep-branch",
				Usage: "Remove worktree but keep the local branch",
			},
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Skip confirmation prompt",
			},
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target a willow-managed repo by name",
			},
			&cli.BoolFlag{
				Name:  "prune",
				Usage: "Run git worktree prune after removal",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			g := flags.NewGit()
			u := flags.NewUI()

			var bareDir string
			var err error
			if repoFlag := cmd.String("repo"); repoFlag != "" {
				bareDir, err = config.ResolveRepo(repoFlag)
				if err != nil {
					return err
				}
			} else {
				bareDir, err = requireWillowRepo(g)
				if err != nil {
					return err
				}
			}

			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			worktrees, err := worktree.List(repoGit)
			if err != nil {
				return fmt.Errorf("failed to list worktrees: %w", err)
			}

			filtered := filterBareWorktrees(worktrees)

			target := cmd.StringArg("branch")
			var wt *worktree.Worktree

			if target == "" {
				// No argument: launch fzf picker
				repoName := repoNameFromDir(bareDir)
				selectedPath, err := fzfPickWorktree(filtered, repoName)
				if err != nil {
					return err
				}
				if selectedPath == "" {
					return nil
				}
				// Find the worktree matching the selected path
				for i := range filtered {
					if filtered[i].Path == selectedPath {
						wt = &filtered[i]
						break
					}
				}
				if wt == nil {
					return fmt.Errorf("selected worktree not found")
				}
			} else {
				wt, err = findWorktree(filtered, target)
				if err != nil {
					return err
				}
			}

			force := cmd.Bool("force")
			wtGit := &git.Git{Dir: wt.Path, Verbose: g.Verbose}

			if !force {
				dirty, err := wtGit.IsDirty()
				if err != nil {
					return err
				}
				if dirty {
					u.Warn(fmt.Sprintf("Worktree %s has uncommitted changes", u.Bold(wt.Branch)))
				}

				unpushed, err := wtGit.HasUnpushedCommits()
				if err != nil {
					return err
				}
				if unpushed {
					u.Warn(fmt.Sprintf("Worktree %s has unpushed commits", u.Bold(wt.Branch)))
				}

				if (dirty || unpushed) && !cmd.Bool("yes") {
					if !confirm("Remove anyway?") {
						u.Info("Aborted.")
						return nil
					}
				} else if !cmd.Bool("yes") {
					if !confirm(fmt.Sprintf("Remove worktree %s?", wt.Branch)) {
						u.Info("Aborted.")
						return nil
					}
				}
			}

			// Run teardown hooks before removal
			cfg := config.Load(bareDir)
			if len(cfg.Teardown) > 0 {
				u.Info("Running teardown hooks...")
				if err := runHooks(cfg.Teardown, wt.Path, u); err != nil {
					return err
				}
			}

			if _, err := repoGit.Run("worktree", "remove", "--force", wt.Path); err != nil {
				return fmt.Errorf("failed to remove worktree: %w", err)
			}

			if !cmd.Bool("keep-branch") {
				if _, err := repoGit.Run("branch", "-D", wt.Branch); err != nil {
					u.Warn(fmt.Sprintf("Failed to delete branch %s: %v", wt.Branch, err))
				}
			}

			// Clean up status file
			repoName := repoNameFromDir(bareDir)
			wtDir := filepath.Base(wt.Path)
			statusFile := filepath.Join(claude.StatusDir(), repoName, wtDir+".json")
			os.Remove(statusFile)

			u.Success(fmt.Sprintf("Removed worktree %s", u.Bold(wt.Branch)))

			if cmd.Bool("prune") {
				u.Info("Pruning stale worktrees...")
				if _, err := repoGit.Run("worktree", "prune"); err != nil {
					u.Warn(fmt.Sprintf("Prune failed: %v", err))
				} else {
					u.Success("Pruned stale worktrees")
				}
			}

			return nil
		},
	}
}

func confirm(prompt string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}
