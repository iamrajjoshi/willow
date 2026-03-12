package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/ui"
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
			tr := trace.New(flags.Trace)
			g := flags.NewGit()
			u := flags.NewUI()

			done := tr.Start("resolve repo")
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
			done()

			done = tr.Start("list worktrees")
			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			worktrees, err := worktree.List(repoGit)
			if err != nil {
				return fmt.Errorf("failed to list worktrees: %w", err)
			}
			done()

			filtered := filterBareWorktrees(worktrees)

			done = tr.Start("resolve targets")
			target := cmd.StringArg("branch")
			var targets []worktree.Worktree

			if target == "" {
				repoName := repoNameFromDir(bareDir)
				selectedPaths, err := fzfPickWorktrees(filtered, repoName)
				if err != nil {
					return err
				}
				if selectedPaths == nil {
					return nil
				}
				for _, sp := range selectedPaths {
					for i := range filtered {
						if filtered[i].Path == sp {
							targets = append(targets, filtered[i])
							break
						}
					}
				}
				if len(targets) == 0 {
					return fmt.Errorf("selected worktrees not found")
				}
			} else {
				wt, err := findWorktree(filtered, target)
				if err != nil {
					return err
				}
				targets = []worktree.Worktree{*wt}
			}
			done()

			force := cmd.Bool("force")
			keepBranch := cmd.Bool("keep-branch")

			done = tr.Start("load config")
			cfg := config.Load(bareDir)
			done()

			for _, wt := range targets {
				if err := removeWorktree(tr, u, repoGit, &wt, bareDir, cfg, force, keepBranch, g.Verbose); err != nil {
					return err
				}
			}

			if cmd.Bool("prune") {
				done = tr.Start("git worktree prune")
				u.Info("Pruning stale worktrees...")
				if _, err := repoGit.Run("worktree", "prune"); err != nil {
					u.Warn(fmt.Sprintf("Prune failed: %v", err))
				} else {
					u.Success("Pruned stale worktrees")
				}
				done()
			}

			tr.Total()
			return nil
		},
	}
}

func removeWorktree(tr *trace.Tracer, u *ui.UI, repoGit *git.Git, wt *worktree.Worktree, bareDir string, cfg *config.Config, force, keepBranch, verbose bool) error {
	wtGit := &git.Git{Dir: wt.Path, Verbose: verbose}

	if !force {
		done := tr.Start("check dirty " + wt.Branch)
		dirty, err := wtGit.IsDirty()
		if err != nil {
			return err
		}
		if dirty {
			u.Warn(fmt.Sprintf("Worktree %s has uncommitted changes", u.Bold(wt.Branch)))
		}
		done()

		done = tr.Start("check unpushed " + wt.Branch)
		unpushed, err := wtGit.HasUnpushedCommits()
		if err != nil {
			return err
		}
		if unpushed {
			u.Warn(fmt.Sprintf("Worktree %s has unpushed commits", u.Bold(wt.Branch)))
		}
		done()
	}

	if len(cfg.Teardown) > 0 {
		done := tr.Start("teardown hooks " + wt.Branch)
		u.Info(fmt.Sprintf("Running teardown hooks for %s...", u.Bold(wt.Branch)))
		if err := runHooks(cfg.Teardown, wt.Path, u); err != nil {
			return err
		}
		done()
	}

	done := tr.Start("remove worktree " + wt.Branch)
	adminDir := filepath.Join(bareDir, "worktrees", filepath.Base(wt.Path))
	if err := os.RemoveAll(adminDir); err != nil {
		return fmt.Errorf("failed to remove worktree admin dir: %w", err)
	}

	trashDir := config.TrashDir()
	if err := os.MkdirAll(trashDir, 0o755); err != nil {
		return fmt.Errorf("failed to create trash dir: %w", err)
	}

	trashDest := filepath.Join(trashDir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), filepath.Base(wt.Path)))
	if err := os.Rename(wt.Path, trashDest); err != nil {
		if removeErr := os.RemoveAll(wt.Path); removeErr != nil {
			return fmt.Errorf("failed to remove worktree: %w", removeErr)
		}
	} else {
		bgRm := exec.Command("rm", "-rf", trashDest)
		bgRm.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		_ = bgRm.Start()
	}
	done()

	done = tr.Start("git branch -D " + wt.Branch)
	if !keepBranch {
		if _, err := repoGit.Run("branch", "-D", wt.Branch); err != nil {
			u.Warn(fmt.Sprintf("Failed to delete branch %s: %v", wt.Branch, err))
		}
	}
	done()

	done = tr.Start("cleanup status " + wt.Branch)
	repoName := repoNameFromDir(bareDir)
	wtDir := filepath.Base(wt.Path)
	claude.RemoveStatusDir(repoName, wtDir)
	done()

	u.Success(fmt.Sprintf("Removed worktree %s", u.Bold(wt.Branch)))
	return nil
}
