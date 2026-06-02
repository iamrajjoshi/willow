package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iamrajjoshi/willow/internal/cleanup"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/trace"
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
				Usage: "Interactively remove safe stale worktrees",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would be cleaned up without removing anything",
			},
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target a willow-managed repo by name",
			},
			&cli.BoolFlag{
				Name:  "no-fetch",
				Usage: "Skip fetching and pruning remotes before scanning",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.gc")()
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

			repos, err := gcRepos(cmd.String("repo"))
			if err != nil {
				return err
			}
			var candidates []cleanup.Candidate
			for _, repo := range repos {
				cfg := config.Load(repo.BareDir)
				repoGit := &git.Git{Dir: repo.BareDir, Verbose: flags.Verbose}
				if *cfg.Defaults.Fetch && !cmd.Bool("no-fetch") {
					if _, err := repoGit.Run("fetch", "--prune", "origin"); err != nil {
						u.Warn(fmt.Sprintf("Skipping remote refresh for %s: %v", repo.Name, err))
					}
				}

				repoCandidates, err := cleanup.ScanRepo(repo.Name, repo.BareDir, cleanup.ScanOptions{
					RefreshPRState: true,
					Verbose:        flags.Verbose,
				})
				if err != nil {
					u.Warn(fmt.Sprintf("Skipping repo %s: %v", repo.Name, err))
					continue
				}
				candidates = append(candidates, repoCandidates...)
			}

			if len(candidates) == 0 {
				u.Info("No stale worktrees found.")
				return nil
			}

			multiRepo := cleanup.HasMultipleRepos(candidates)
			u.Info(fmt.Sprintf("\nFound %d stale worktree(s):", len(candidates)))
			for _, c := range candidates {
				u.Info(fmt.Sprintf("  %s (%s)", cleanup.Label(c, multiRepo), c.ReasonString()))
			}

			if !prune {
				u.Info("\nTo remove these, run:")
				for _, c := range candidates {
					u.Info(fmt.Sprintf("  willow rm %s --repo %s", c.Branch, c.RepoName))
				}
				u.Info("\nOr re-run with --prune to interactively remove the safe subset.")
				return nil
			}

			safe, skipped, err := cleanup.FilterSafe(candidates)
			if err != nil {
				return err
			}
			if len(skipped) > 0 {
				u.Info("\nSkipping unsafe stale worktrees:")
				for _, skip := range skipped {
					u.Info(fmt.Sprintf("  %s (%s)", cleanup.Label(skip.Candidate, multiRepo), skip.Reason))
				}
			}

			if dryRun {
				u.Info(fmt.Sprintf("\nDry run: would remove %d safe stale worktree(s).", len(safe)))
				return nil
			}

			if len(safe) == 0 {
				u.Info("\nNo safe stale worktrees to remove.")
				return nil
			}

			fmt.Fprintf(os.Stderr, "\nRemove %d safe stale worktree(s)? [y/N] ", len(safe))
			var answer string
			fmt.Fscanf(os.Stdin, "%s", &answer)
			if answer != "y" && answer != "Y" {
				u.Info("Aborted.")
				return nil
			}

			tr := trace.FromContext(ctx)
			for _, c := range safe {
				u.Info(fmt.Sprintf("Removing %s from %s...", c.Branch, c.RepoName))
				repoGit := &git.Git{Dir: c.BareDir, Verbose: flags.Verbose}
				cfg := config.Load(c.BareDir)
				wt := worktree.Worktree{
					Branch: c.Branch,
					Path:   c.Path,
					Head:   c.Head,
				}
				if err := removeWorktree(ctx, tr, u, repoGit, &wt, c.BareDir, cfg, true, false, flags.Verbose); err != nil {
					u.Warn(fmt.Sprintf("Failed to remove %s: %v", c.Branch, err))
				}
			}

			return nil
		},
	}
}

func gcRepos(repoFlag string) ([]repoInfo, error) {
	if repoFlag != "" {
		bareDir, err := config.ResolveRepo(repoFlag)
		if err != nil {
			return nil, err
		}
		return []repoInfo{{Name: repoFlag, BareDir: bareDir}}, nil
	}

	repoNames, err := config.ListRepos()
	if err != nil {
		return nil, fmt.Errorf("failed to list repos: %w", err)
	}

	repos := make([]repoInfo, 0, len(repoNames))
	for _, repoName := range repoNames {
		bareDir, err := config.ResolveRepo(repoName)
		if err != nil {
			continue
		}
		repos = append(repos, repoInfo{Name: repoName, BareDir: bareDir})
	}
	return repos, nil
}
