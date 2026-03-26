package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errs"
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
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			tr := trace.New(flags.Trace)
			g := flags.NewGit()
			u := flags.NewUI()
			cdOnly := cmd.Bool("cd")

			branch := cmd.StringArg("branch")

			// Resolve repos
			done := tr.Start("resolve repos")
			repos, err := resolveRepos(g, cmd.String("repo"))
			if err != nil {
				return err
			}
			done()

			// --pr flag: resolve PR number or URL to branch name
			if prRef := cmd.String("pr"); prRef != "" {
				done = tr.Start("resolve PR")
				prBranch, ok := resolvePRRef(prRef, repos[0].BareDir)
				if !ok {
					return errs.Userf("failed to resolve PR: %s\n\nEnsure 'gh' is installed and you're authenticated", prRef)
				}
				if cdOnly {
					fmt.Fprintf(os.Stderr, "Resolved PR to branch %s\n", prBranch)
				} else {
					u.Info(fmt.Sprintf("Resolved PR to branch %s", u.Bold(prBranch)))
				}
				branch = prBranch
				done()
			}

			// PR URL auto-detection in branch arg
			if branch != "" && isPRURL(branch) {
				done = tr.Start("resolve PR branch")
				prBranch, ok := resolvePRRef(branch, repos[0].BareDir)
				if !ok {
					return errs.Userf("failed to resolve branch from PR URL: %s\n\nEnsure 'gh' is installed and you're authenticated", branch)
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
				return errs.Userf("branch name or PR URL is required\n\nUsage: ww checkout <branch-or-pr-url>")
			}

			// Step 1: Check if a worktree already exists for this branch
			done = tr.Start("find existing worktree")
			allWts := collectAllWorktrees(repos, g.Verbose)
			rwt, _ := findCrossRepoWorktree(allWts, branch)
			done()

			if rwt != nil {
				// Switch mode — worktree exists
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

			// Step 2: No worktree found — need to create one.
			// Resolve to a single repo.
			done = tr.Start("resolve single repo")
			if len(repos) > 1 {
				return errs.Userf("multiple repos found — use --repo to specify which one")
			}
			repo := repos[0]
			done()

			done = tr.Start("load config")
			cfg := config.Load(repo.BareDir)
			done()

			repoGit := &git.Git{Dir: repo.BareDir, Verbose: g.Verbose}

			// Fetch to ensure we have the latest remote refs
			shouldFetch := *cfg.Defaults.Fetch && !cmd.Bool("no-fetch")

			// Step 3: Check if branch exists on remote
			if shouldFetch {
				done = tr.Start("git fetch")
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
				// Existing branch — create worktree for it
				dirName := strings.ReplaceAll(branch, "/", "-")
				wtPath := filepath.Join(config.WorktreesDir(), repo.Name, dirName)

				done = tr.Start("git worktree add (existing)")
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

				return finishWorktree(tr, cfg, g, u, wtPath, repo.Name, branch, "", cdOnly)
			}

			// New branch — apply prefix, resolve base, create
			if cfg.BranchPrefix != "" && !strings.HasPrefix(branch, cfg.BranchPrefix+"/") {
				branch = cfg.BranchPrefix + "/" + branch
			}

			done = tr.Start("resolve base branch")
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

			// Local branch as base (for stacked PRs) or remote
			localBase := repoGit.LocalBranchExists(baseBranch)
			gitRef := "origin/" + baseBranch
			if localBase {
				gitRef = baseBranch
			}

			dirName := strings.ReplaceAll(branch, "/", "-")
			wtPath := filepath.Join(config.WorktreesDir(), repo.Name, dirName)

			done = tr.Start("git worktree add (new)")
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

			// Record parent in stack
			done = tr.Start("record stack parent")
			st := stack.Load(repo.BareDir)
			st.SetParent(branch, baseBranch)
			if err := st.Save(repo.BareDir); err != nil {
				u.Warn(fmt.Sprintf("Failed to save stack: %v", err))
			}
			done()

			return finishWorktree(tr, cfg, g, u, wtPath, repo.Name, branch, baseBranch, cdOnly)
		},
	}
}
