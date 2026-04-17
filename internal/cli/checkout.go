package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errors"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/urfave/cli/v3"
)

func checkoutCmd() *cli.Command {
	return &cli.Command{
		Name:          "checkout",
		Aliases:       []string{"co"},
		Usage:         "Switch to a worktree, or create one if it doesn't exist",
		ShellComplete: completeWorktreesWithFlag,
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "branch",
				UsageText: "<branch-or-pr-url>",
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target a willow-managed repo by name",
			},
			&cli.StringFlag{
				Name:    "base",
				Aliases: []string{"b"},
				Usage:   "Base branch (only when creating a new branch)",
			},
			&cli.BoolFlag{
				Name:  "no-fetch",
				Usage: "Skip fetching latest from remote",
			},
			&cli.BoolFlag{
				Name:  "cd",
				Usage: "Print only the worktree path to stdout",
			},
			&cli.StringFlag{
				Name:  "pr",
				Usage: "GitHub PR number or URL",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			tr := trace.FromContext(ctx)
			defer tr.Total()
			g := flags.NewGit()
			u := flags.NewUI()
			cdOnly := cmd.Bool("cd")
			if cdOnly {
				u.Out = os.Stderr
			}

			branch := cmd.StringArg("branch")

			done := tr.StartCtx(ctx, "resolve repos")
			repos, err := resolveRepos(g, cmd.String("repo"))
			if err != nil {
				return err
			}
			done()

			if prRef := cmd.String("pr"); prRef != "" {
				done = tr.StartCtx(ctx, "resolve PR")
				prBranch, err := resolvePRRef(prRef, repos[0].BareDir)
				if err != nil {
					return errors.User(fmt.Errorf("failed to resolve PR %s: %w", prRef, err))
				}
				if cdOnly {
					fmt.Fprintf(os.Stderr, "Resolved PR to branch %s\n", prBranch)
				} else {
					u.Info(fmt.Sprintf("Resolved PR to branch %s", u.Bold(prBranch)))
				}
				branch = prBranch
				done()
			}

			if branch != "" && isPRURL(branch) {
				done = tr.StartCtx(ctx, "resolve PR branch")
				prBranch, err := resolvePRRef(branch, repos[0].BareDir)
				if err != nil {
					return errors.User(fmt.Errorf("failed to resolve PR from URL %s: %w", branch, err))
				}
				if cdOnly {
					fmt.Fprintf(os.Stderr, "Resolved PR to branch %s\n", prBranch)
				} else {
					u.Info(fmt.Sprintf("Resolved PR to branch %s", u.Bold(prBranch)))
				}
				branch = prBranch
				done()
			}

			if branch == "" {
				return errors.Userf("branch name or PR URL is required\n\nUsage: ww checkout <branch-or-pr-url>")
			}

			done = tr.StartCtx(ctx, "find existing worktree")
			allWts := collectAllWorktrees(repos, g.Verbose)
			rwt, _ := findCrossRepoWorktree(allWts, branch)
			done()

			if rwt != nil {
				wtDir := filepath.Base(rwt.Worktree.Path)
				claude.MarkRead(rwt.Repo.Name, wtDir)

				if cdOnly {
					fmt.Println(rwt.Worktree.Path)
					return nil
				}

				u.Success(fmt.Sprintf("Switched to %s", u.Bold(branch)))
				u.Info(fmt.Sprintf("  path:   %s", u.Dim(rwt.Worktree.Path)))
				fmt.Println(rwt.Worktree.Path)
				return nil
			}

			done = tr.StartCtx(ctx, "resolve single repo")
			if len(repos) > 1 {
				return errors.Userf("multiple repos found — use --repo to specify which one")
			}
			repo := repos[0]
			done()

			done = tr.StartCtx(ctx, "load config")
			cfg := config.Load(repo.BareDir)
			done()

			repoGit := &git.Git{Dir: repo.BareDir, Verbose: g.Verbose}

			shouldFetch := *cfg.Defaults.Fetch && !cmd.Bool("no-fetch")

			if shouldFetch {
				done = tr.StartCtx(ctx, "git fetch")
				if cdOnly {
					fmt.Fprintf(os.Stderr, "Fetching from origin...\n")
					repoGit.RunStream(os.Stderr, "fetch", "--progress", "origin")
				} else {
					u.Info("Fetching from origin...")
					repoGit.Run("fetch", "origin")
				}
				done()
			}

			if repoGit.RemoteBranchExists(branch) {
				dirName := strings.ReplaceAll(branch, "/", "-")
				wtPath := filepath.Join(config.WorktreesDir(), repo.Name, dirName)

				done = tr.StartCtx(ctx, "git worktree add (existing)")
				if cdOnly {
					fmt.Fprintf(os.Stderr, "Creating worktree for existing branch %s...\n", branch)
					if _, err := repoGit.RunStream(os.Stderr, "worktree", "add", wtPath, branch); err != nil {
						return fmt.Errorf("failed to create worktree: %w", err)
					}
				} else {
					u.Info(fmt.Sprintf("Creating worktree for existing branch %s...", u.Bold(branch)))
					if _, err := repoGit.Run("worktree", "add", wtPath, branch); err != nil {
						return fmt.Errorf("failed to create worktree: %w", err)
					}
				}
				done()

				return finishWorktree(ctx, tr, cfg, g, u, wtPath, repo.Name, branch, "", cdOnly)
			}

			if cfg.BranchPrefix != "" && !strings.HasPrefix(branch, cfg.BranchPrefix+"/") {
				branch = cfg.BranchPrefix + "/" + branch
			}

			done = tr.StartCtx(ctx, "resolve base branch")
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
			done()

			localBase := repoGit.LocalBranchExists(baseBranch)
			gitRef := "origin/" + baseBranch
			if localBase {
				gitRef = baseBranch
			}

			dirName := strings.ReplaceAll(branch, "/", "-")
			wtPath := filepath.Join(config.WorktreesDir(), repo.Name, dirName)

			done = tr.StartCtx(ctx, "git worktree add (new)")
			if cdOnly {
				fmt.Fprintf(os.Stderr, "Creating worktree %s from %s...\n", branch, gitRef)
				if _, err := repoGit.RunStream(os.Stderr, "worktree", "add", wtPath, "-b", branch, gitRef); err != nil {
					return fmt.Errorf("failed to create worktree: %w", err)
				}
			} else {
				u.Info(fmt.Sprintf("Creating worktree %s from %s...", u.Bold(branch), u.Bold(gitRef)))
				if _, err := repoGit.Run("worktree", "add", wtPath, "-b", branch, gitRef); err != nil {
					return fmt.Errorf("failed to create worktree: %w", err)
				}
			}
			done()

			done = tr.StartCtx(ctx, "record stack parent")
			if err := stack.Update(repo.BareDir, func(s *stack.Stack) {
				s.SetParent(branch, baseBranch)
			}); err != nil {
				u.Warn(fmt.Sprintf("Failed to save stack: %v", err))
			}
			done()

			return finishWorktree(ctx, tr, cfg, g, u, wtPath, repo.Name, branch, baseBranch, cdOnly)
		},
	}
}
