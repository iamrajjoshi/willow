package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func gcCmd() *cli.Command {
	return &cli.Command{
		Name:  "gc",
		Usage: "Clean up leftover trash from removed worktrees",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "prune",
				Usage: "Interactively remove merged worktrees",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would be cleaned up without removing anything",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			u := flags.NewUI()
			dryRun := cmd.Bool("dry-run")
			prune := cmd.Bool("prune")

			trashDir := config.TrashDir()
			entries, err := os.ReadDir(trashDir)
			if err != nil {
				if os.IsNotExist(err) {
					u.Info("No trash to clean up.")
				} else {
					return fmt.Errorf("failed to read trash dir: %w", err)
				}
			} else if len(entries) == 0 {
				u.Info("No trash to clean up.")
			} else if dryRun {
				u.Info(fmt.Sprintf("Would clean up %d trash entries", len(entries)))
			} else {
				for _, e := range entries {
					path := filepath.Join(trashDir, e.Name())
					if err := os.RemoveAll(path); err != nil {
						u.Warn(fmt.Sprintf("Failed to remove %s: %v", e.Name(), err))
					}
				}
				u.Success(fmt.Sprintf("Cleaned up %d trash entries", len(entries)))
			}

			repos, err := config.ListRepos()
			if err != nil {
				return fmt.Errorf("failed to list repos: %w", err)
			}

			type mergedWorktree struct {
				repoName string
				branch   string
			}
			var candidates []mergedWorktree

			for _, repoName := range repos {
				bareDir, err := config.ResolveRepo(repoName)
				if err != nil {
					u.Warn(fmt.Sprintf("Skipping repo %s: %v", repoName, err))
					continue
				}

				cfg := config.Load(bareDir)
				repoGit := &git.Git{Dir: bareDir}
				merged, err := repoGit.MergedBranches(cfg.ResolveBaseBranch())
				if err != nil {
					u.Warn(fmt.Sprintf("Skipping repo %s: failed to get merged branches: %v", repoName, err))
					continue
				}
				if len(merged) == 0 {
					continue
				}

				wts, err := worktree.List(repoGit)
				if err != nil {
					u.Warn(fmt.Sprintf("Skipping repo %s: failed to list worktrees: %v", repoName, err))
					continue
				}

				wtBranches := make(map[string]bool)
				for _, wt := range wts {
					if !wt.IsBare {
						wtBranches[wt.Branch] = true
					}
				}

				for _, branch := range merged {
					if wtBranches[branch] {
						candidates = append(candidates, mergedWorktree{repoName: repoName, branch: branch})
					}
				}
			}

			if len(candidates) == 0 {
				u.Info("No merged worktrees found.")
				return nil
			}

			u.Info(fmt.Sprintf("\nFound %d merged worktree(s):", len(candidates)))
			for _, c := range candidates {
				u.Info(fmt.Sprintf("  %s (repo: %s)", c.branch, c.repoName))
			}

			if !prune {
				u.Info("\nTo remove these, run:")
				for _, c := range candidates {
					u.Info(fmt.Sprintf("  willow rm %s --repo %s", c.branch, c.repoName))
				}
				u.Info("\nOr re-run with --prune to interactively remove them.")
				return nil
			}

			if dryRun {
				u.Info("\nDry run: would remove the above worktrees.")
				return nil
			}

			fmt.Fprintf(os.Stderr, "\nRemove %d merged worktree(s)? [y/N] ", len(candidates))
			var answer string
			fmt.Fscanf(os.Stdin, "%s", &answer)
			if answer != "y" && answer != "Y" {
				u.Info("Aborted.")
				return nil
			}

			self, err := os.Executable()
			if err != nil {
				self = "willow"
			}

			for _, c := range candidates {
				u.Info(fmt.Sprintf("Removing %s from %s...", c.branch, c.repoName))
				rmCmd := exec.Command(self, "rm", c.branch, "--force", "--repo", c.repoName)
				rmCmd.Stdout = os.Stdout
				rmCmd.Stderr = os.Stderr
				if err := rmCmd.Run(); err != nil {
					u.Warn(fmt.Sprintf("Failed to remove %s: %v", c.branch, err))
				}
			}

			return nil
		},
	}
}
