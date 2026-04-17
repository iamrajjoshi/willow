package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errs"
	"github.com/iamrajjoshi/willow/internal/fzf"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/log"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/ui"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func repoNameFromDir(bareDir string) string {
	return strings.TrimSuffix(filepath.Base(bareDir), ".git")
}

func runHooks(commands []string, dir string, u *ui.UI, stdout *os.File) error {
	for _, c := range commands {
		u.Info(fmt.Sprintf("  → %s", c))
		sh := exec.Command("sh", "-c", c)
		sh.Dir = dir
		sh.Stdout = stdout
		sh.Stderr = os.Stderr
		if err := sh.Run(); err != nil {
			return fmt.Errorf("hook failed: %s: %w", c, err)
		}
	}
	return nil
}

// runPostCheckoutHook manually invokes the configured post-checkout hook from
// the new worktree. This is needed for bare repos because git resolves relative
// core.hooksPath against the bare repo dir (where hook files don't exist),
// so the hook never fires automatically.
func runPostCheckoutHook(hookPath, wtPath string, u *ui.UI, cdOnly bool) {
	if hookPath == "" {
		return
	}

	hookFile := filepath.Join(wtPath, hookPath)
	if _, err := os.Stat(hookFile); err != nil {
		return
	}

	wtGit := &git.Git{Dir: wtPath}
	head, err := wtGit.Run("rev-parse", "HEAD")
	if err != nil {
		u.Warn(fmt.Sprintf("post-checkout hook: failed to resolve HEAD: %v", err))
		return
	}

	nullRef := "0000000000000000000000000000000000000000"
	cmd := exec.Command(hookFile, nullRef, head, "1")
	cmd.Dir = wtPath
	// In --cd mode stdout is captured for the worktree path, so redirect
	// hook output to stderr to avoid contaminating it.
	if cdOnly {
		cmd.Stdout = os.Stderr
	} else {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		u.Warn(fmt.Sprintf("post-checkout hook failed: %v", err))
	}
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
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target a willow-managed repo by name",
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
			existing := cmd.Bool("existing")

			var bareDir string
			var err error
			done := tr.StartCtx(ctx, "resolve repo")
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

			done = tr.StartCtx(ctx, "load config")
			cfg := config.Load(bareDir)
			done()

			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			repoName := repoNameFromDir(bareDir)

			branch := cmd.StringArg("branch")

			if prRef := cmd.String("pr"); prRef != "" {
				done = tr.StartCtx(ctx, "resolve PR")
				prBranch, err := resolvePRRef(prRef, bareDir)
				if err != nil {
					return errs.User(fmt.Errorf("failed to resolve PR %s: %w", prRef, err))
				}
				if cdOnly {
					fmt.Fprintf(os.Stderr, "Resolved PR to branch %s\n", prBranch)
				} else {
					u.Info(fmt.Sprintf("Resolved PR to branch %s", u.Bold(prBranch)))
				}
				branch = prBranch
				existing = true
				done()
			}

			if branch != "" && isPRURL(branch) {
				done = tr.StartCtx(ctx, "resolve PR branch")
				prBranch, err := resolvePRRef(branch, bareDir)
				if err != nil {
					return errs.User(fmt.Errorf("failed to resolve PR from URL %s: %w", branch, err))
				}
				if cdOnly {
					fmt.Fprintf(os.Stderr, "Resolved PR to branch %s\n", prBranch)
				} else {
					u.Info(fmt.Sprintf("Resolved PR to branch %s", u.Bold(prBranch)))
				}
				branch = prBranch
				existing = true
				done()
			}

			if existing && branch == "" {
				shouldFetch := *cfg.Defaults.Fetch && !cmd.Bool("no-fetch")
				if shouldFetch {
					u.Info("Fetching latest branches from origin...")
					if _, err := repoGit.Run("fetch", "origin"); err != nil {
						u.Warn(fmt.Sprintf("Failed to fetch: %v", err))
					}
				}
				done = tr.StartCtx(ctx, "pick existing branch")
				branch, err = pickExistingBranch(repoGit)
				if err != nil {
					return err
				}
				if branch == "" {
					return nil // user cancelled
				}
				done()
			}

			if branch == "" {
				return errs.Userf("branch name is required\n\nUsage: ww new <branch> [flags]")
			}

			if existing {
				shouldFetch := *cfg.Defaults.Fetch && !cmd.Bool("no-fetch")
				if shouldFetch {
					done = tr.StartCtx(ctx, "git fetch branch")
					if cdOnly {
						fmt.Fprintf(os.Stderr, "Fetching %s from origin...\n", branch)
						if _, err := repoGit.RunStream(os.Stderr, "fetch", "--progress", "origin", branch); err != nil {
							return fmt.Errorf("failed to fetch origin/%s: %w", branch, err)
						}
					} else {
						u.Info(fmt.Sprintf("Fetching %s from origin...", u.Bold(branch)))
						if _, err := repoGit.Run("fetch", "origin", branch); err != nil {
							return fmt.Errorf("failed to fetch origin/%s: %w", branch, err)
						}
					}
					done()
				}

				dirName := strings.ReplaceAll(branch, "/", "-")
				wtPath := filepath.Join(config.WorktreesDir(), repoName, dirName)

				done = tr.StartCtx(ctx, "git worktree add")
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

				return finishWorktree(ctx, tr, cfg, g, u, wtPath, repoName, branch, "", cdOnly)
			}

			if cfg.BranchPrefix != "" && !strings.HasPrefix(branch, cfg.BranchPrefix+"/") {
				branch = cfg.BranchPrefix + "/" + branch
			}

			done = tr.StartCtx(ctx, "resolve base branch")
			baseBranch := cmd.String("base")
			explicitBase := baseBranch != ""
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

			// Only treat as local base when --base was explicitly provided and the
			// branch exists locally (stacked PRs). Auto-detected defaults always
			// use origin/ so they stay current — in bare repos every branch appears
			// local, which previously caused the fetch to be skipped.
			localBase := explicitBase && repoGit.LocalBranchExists(baseBranch)
			gitRef := "origin/" + baseBranch
			if localBase {
				gitRef = baseBranch
			}

			shouldFetch := *cfg.Defaults.Fetch && !cmd.Bool("no-fetch") && !localBase
			if shouldFetch {
				done = tr.StartCtx(ctx, "git fetch")
				if cdOnly {
					fmt.Fprintf(os.Stderr, "Fetching %s from origin...\n", baseBranch)
					if _, err := repoGit.RunStream(os.Stderr, "fetch", "--progress", "origin", baseBranch); err != nil {
						return fmt.Errorf("failed to fetch origin/%s: %w", baseBranch, err)
					}
				} else {
					u.Info(fmt.Sprintf("Fetching %s from origin...", u.Bold(baseBranch)))
					if _, err := repoGit.Run("fetch", "origin", baseBranch); err != nil {
						return fmt.Errorf("failed to fetch origin/%s: %w", baseBranch, err)
					}
				}
				done()
			}

			dirName := strings.ReplaceAll(branch, "/", "-")
			wtPath := filepath.Join(config.WorktreesDir(), repoName, dirName)

			done = tr.StartCtx(ctx, "git worktree add")
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
			if err := stack.Update(bareDir, func(s *stack.Stack) {
				s.SetParent(branch, baseBranch)
			}); err != nil {
				u.Warn(fmt.Sprintf("Failed to save stack: %v", err))
			}
			done()

			return finishWorktree(ctx, tr, cfg, g, u, wtPath, repoName, branch, baseBranch, cdOnly)
		},
	}
}

func finishWorktree(ctx context.Context, tr *trace.Tracer, cfg *config.Config, g *git.Git, u *ui.UI, wtPath, repoName, branch, baseBranch string, cdOnly bool) error {
	done := tr.StartCtx(ctx, "post-checkout hook")
	runPostCheckoutHook(cfg.PostCheckoutHook, wtPath, u, cdOnly)
	done()

	done = tr.StartCtx(ctx, "auto setup remote")
	if *cfg.Defaults.AutoSetupRemote {
		wtGit := &git.Git{Dir: wtPath, Verbose: g.Verbose}
		if _, err := wtGit.Run("config", "--local", "push.autoSetupRemote", "true"); err != nil {
			u.Warn("Failed to set push.autoSetupRemote: " + err.Error())
		}
	}
	done()

	hookOut := os.Stdout
	if cdOnly {
		hookOut = os.Stderr
	}

	done = tr.StartCtx(ctx, "setup hooks")
	if len(cfg.Setup) > 0 {
		u.Info("Running setup hooks...")
		if err := runHooks(cfg.Setup, wtPath, u, hookOut); err != nil {
			return err
		}
	}
	done()

	done = tr.StartCtx(ctx, "pane commands")
	if !cdOnly {
		for _, w := range cfg.Validate() {
			u.Warn(w)
		}
	}
	if len(cfg.Tmux.Panes) > 0 && !cdOnly {
		for i, p := range cfg.Tmux.Panes {
			if p.Command != "" {
				if err := runHooks([]string{p.Command}, wtPath, u, hookOut); err != nil {
					return errs.User(fmt.Errorf("pane %d command failed: %w", i, err))
				}
			}
		}
	}
	done()

	meta := map[string]string{}
	if baseBranch != "" {
		meta["base"] = baseBranch
	}
	_ = log.Append(log.Event{Action: "create", Repo: repoName, Branch: branch, Metadata: meta})

	if cdOnly {
		fmt.Println(wtPath)
		return nil
	}

	u.Success(fmt.Sprintf("Created worktree %s", u.Bold(branch)))
	u.Info(fmt.Sprintf("  path:   %s", u.Dim(wtPath)))
	if baseBranch != "" {
		u.Info(fmt.Sprintf("  base:   %s", u.Dim("origin/"+baseBranch)))
	}
	return nil
}

var prURLPattern = regexp.MustCompile(`github\.com/.+/pull/\d+`)

func isPRURL(s string) bool {
	return prURLPattern.MatchString(s)
}

// resolvePRRef resolves a PR reference (number like "123" or full URL) to a branch name.
// Uses `gh pr view` which handles both forms natively.
func resolvePRRef(input, bareDir string) (string, error) {
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return "", fmt.Errorf("'gh' CLI not found — install it from https://cli.github.com")
	}

	cmd := exec.Command(ghPath, "pr", "view", input, "--json", "headRefName", "-q", ".headRefName")
	cmd.Dir = bareDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return "", fmt.Errorf("gh pr view failed: %s", msg)
		}
		return "", fmt.Errorf("gh pr view failed: %w", err)
	}

	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "", fmt.Errorf("gh returned empty branch name for PR %s", input)
	}
	return branch, nil
}

func pickExistingBranch(repoGit *git.Git) (string, error) {
	remoteBranches, err := repoGit.RemoteBranches()
	if err != nil {
		return "", fmt.Errorf("failed to list remote branches: %w", err)
	}
	if len(remoteBranches) == 0 {
		return "", errs.Userf("no remote branches found")
	}

	wts, err := worktree.List(repoGit)
	if err != nil {
		return "", fmt.Errorf("failed to list worktrees: %w", err)
	}
	wtBranches := make(map[string]bool)
	for _, wt := range wts {
		if !wt.IsBare {
			wtBranches[wt.Branch] = true
		}
	}

	var available []string
	for _, b := range remoteBranches {
		if !wtBranches[b] {
			available = append(available, b)
		}
	}
	if len(available) == 0 {
		return "", errs.Userf("all remote branches already have worktrees")
	}

	selected, err := fzf.Run(available,
		fzf.WithReverse(),
		fzf.WithHeader("Select a branch to check out"),
	)
	if err != nil {
		return "", err
	}
	return selected, nil
}
