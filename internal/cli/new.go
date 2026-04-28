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
	"github.com/iamrajjoshi/willow/internal/errors"
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

func detachedWorktreeDirName(name string) (string, error) {
	dirName := worktreeDirName(name)
	if dirName == "" || dirName == "." || dirName == ".." {
		return "", errors.Userf("invalid detached worktree name %q", name)
	}
	return dirName, nil
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

func resolveDetachedRef(ctx context.Context, tr *trace.Tracer, cmd *cli.Command, cfg *config.Config, repoGit *git.Git, u *ui.UI, cdOnly bool) (string, string, error) {
	if ref := cmd.String("ref"); ref != "" {
		return ref, ref, nil
	}

	done := tr.StartCtx(ctx, "resolve detached ref")
	baseBranch := cmd.String("base")
	explicitBase := baseBranch != ""
	var err error
	if baseBranch == "" {
		baseBranch = cfg.BaseBranch
	}
	if baseBranch == "" {
		baseBranch, err = repoGit.DefaultBranch()
		if err != nil {
			done()
			return "", "", fmt.Errorf("failed to detect default branch (use --ref to specify): %w", err)
		}
	}
	done()

	localBase := explicitBase && repoGit.LocalBranchExists(baseBranch)
	gitRef := "origin/" + baseBranch
	if localBase {
		gitRef = baseBranch
	}

	shouldFetch := *cfg.Defaults.Fetch && !cmd.Bool("no-fetch") && !localBase
	if shouldFetch {
		done = tr.StartCtx(ctx, "git fetch detached ref")
		if cdOnly {
			fmt.Fprintf(os.Stderr, "Fetching %s from origin...\n", baseBranch)
			if _, err := repoGit.RunStream(os.Stderr, "fetch", "--progress", "origin", baseBranch); err != nil {
				done()
				return "", "", fmt.Errorf("failed to fetch origin/%s: %w", baseBranch, err)
			}
		} else {
			if err := u.Spin(fmt.Sprintf("Fetching %s from origin", u.Bold(baseBranch)), func() error {
				_, err := repoGit.Run("fetch", "origin", baseBranch)
				return err
			}); err != nil {
				done()
				return "", "", fmt.Errorf("failed to fetch origin/%s: %w", baseBranch, err)
			}
		}
		done()
	}

	return gitRef, gitRef, nil
}

func newCmd() *cli.Command {
	return &cli.Command{
		Name:    "new",
		Aliases: []string{"n"},
		Usage:   "Create a new worktree",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "branch",
				UsageText: "[branch]",
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
				Name:    "detach",
				Aliases: []string{"detached"},
				Usage:   "Create a detached HEAD worktree with an optional name",
			},
			&cli.StringFlag{
				Name:  "ref",
				Usage: "Commit, tag, or branch to check out in detached mode",
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
			detached := cmd.Bool("detach")
			if !detached && cmd.String("ref") != "" {
				return errors.Userf("--ref can only be used with --detach")
			}

			if detached {
				if existing {
					return errors.Userf("--detach cannot be used with --existing")
				}
				if cmd.String("pr") != "" {
					return errors.Userf("--detach cannot be used with --pr")
				}

				ref, refLabel, err := resolveDetachedRef(ctx, tr, cmd, cfg, repoGit, u, cdOnly)
				if err != nil {
					return err
				}

				name := branch
				var dirName string
				if name == "" {
					head, err := resolveDetachedCommit(repoGit, ref)
					if err != nil {
						return err
					}
					dirName = generatedDetachedWorktreeDirName(repoName, head)
					name = dirName
				} else {
					dirName, err = detachedWorktreeDirName(name)
					if err != nil {
						return err
					}
				}

				wtPath := filepath.Join(config.WorktreesDir(), repoName, dirName)

				done = tr.StartCtx(ctx, "git worktree add detached")
				if cdOnly {
					fmt.Fprintf(os.Stderr, "Creating detached worktree %s at %s...\n", name, refLabel)
					if _, err := repoGit.RunStream(os.Stderr, "worktree", "add", "--detach", wtPath, ref); err != nil {
						return fmt.Errorf("failed to create detached worktree: %w", err)
					}
				} else {
					u.Info(fmt.Sprintf("Creating detached worktree %s at %s...", u.Bold(name), u.Bold(refLabel)))
					if _, err := repoGit.Run("worktree", "add", "--detach", wtPath, ref); err != nil {
						return fmt.Errorf("failed to create detached worktree: %w", err)
					}
				}
				done()

				return finishDetachedWorktree(ctx, tr, cfg, g, u, wtPath, repoName, name, refLabel, cdOnly)
			}

			if prRef := cmd.String("pr"); prRef != "" {
				done = tr.StartCtx(ctx, "resolve PR")
				prBranch, err := resolvePRRef(prRef, bareDir)
				if err != nil {
					return errors.User(fmt.Errorf("failed to resolve PR %s: %w", prRef, err))
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
					return errors.User(fmt.Errorf("failed to resolve PR from URL %s: %w", branch, err))
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
					if err := u.Spin("Fetching latest branches from origin", func() error {
						_, err := repoGit.Run("fetch", "origin")
						return err
					}); err != nil {
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
				return errors.Userf("branch name is required\n\nUsage: ww new <branch> [flags]")
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
						if err := u.Spin(fmt.Sprintf("Fetching %s from origin", u.Bold(branch)), func() error {
							_, err := repoGit.Run("fetch", "origin", branch)
							return err
						}); err != nil {
							return fmt.Errorf("failed to fetch origin/%s: %w", branch, err)
						}
					}
					done()
				}

				dirName := worktreeDirName(branch)
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
					if err := u.Spin(fmt.Sprintf("Fetching %s from origin", u.Bold(baseBranch)), func() error {
						_, err := repoGit.Run("fetch", "origin", baseBranch)
						return err
					}); err != nil {
						return fmt.Errorf("failed to fetch origin/%s: %w", baseBranch, err)
					}
				}
				done()
			}

			dirName := worktreeDirName(branch)
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

type finishWorktreeOptions struct {
	BaseBranch string
	Detached   bool
	Ref        string
}

func finishWorktree(ctx context.Context, tr *trace.Tracer, cfg *config.Config, g *git.Git, u *ui.UI, wtPath, repoName, branch, baseBranch string, cdOnly bool) error {
	return finishWorktreeWithOptions(ctx, tr, cfg, g, u, wtPath, repoName, branch, finishWorktreeOptions{BaseBranch: baseBranch}, cdOnly)
}

func finishDetachedWorktree(ctx context.Context, tr *trace.Tracer, cfg *config.Config, g *git.Git, u *ui.UI, wtPath, repoName, name, ref string, cdOnly bool) error {
	return finishWorktreeWithOptions(ctx, tr, cfg, g, u, wtPath, repoName, name, finishWorktreeOptions{Detached: true, Ref: ref}, cdOnly)
}

func finishWorktreeWithOptions(ctx context.Context, tr *trace.Tracer, cfg *config.Config, g *git.Git, u *ui.UI, wtPath, repoName, label string, opts finishWorktreeOptions, cdOnly bool) error {
	done := tr.StartCtx(ctx, "post-checkout hook")
	runPostCheckoutHook(cfg.PostCheckoutHook, wtPath, u, cdOnly)
	done()

	done = tr.StartCtx(ctx, "auto setup remote")
	if *cfg.Defaults.AutoSetupRemote && !opts.Detached {
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
					return errors.User(fmt.Errorf("pane %d command failed: %w", i, err))
				}
			}
		}
	}
	done()

	meta := map[string]string{}
	if opts.BaseBranch != "" {
		meta["base"] = opts.BaseBranch
	}
	if opts.Detached {
		meta["detached"] = "true"
		if opts.Ref != "" {
			meta["ref"] = opts.Ref
		}
	}
	_ = log.Append(log.Event{Action: "create", Repo: repoName, Branch: label, Metadata: meta})

	if cdOnly {
		fmt.Println(wtPath)
		return nil
	}

	if opts.Detached {
		u.Success(fmt.Sprintf("Created detached worktree %s", u.Bold(label)))
	} else {
		u.Success(fmt.Sprintf("Created worktree %s", u.Bold(label)))
	}
	u.Info(fmt.Sprintf("  path:   %s", u.Dim(wtPath)))
	if opts.BaseBranch != "" {
		u.Info(fmt.Sprintf("  base:   %s", u.Dim("origin/"+opts.BaseBranch)))
	}
	if opts.Ref != "" {
		u.Info(fmt.Sprintf("  ref:    %s", u.Dim(opts.Ref)))
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
	return pickExistingBranchWithQuery(repoGit, "")
}

func pickExistingBranchWithQuery(repoGit *git.Git, query string) (string, error) {
	remoteBranches, err := repoGit.RemoteBranches()
	if err != nil {
		return "", fmt.Errorf("failed to list remote branches: %w", err)
	}
	if len(remoteBranches) == 0 {
		return "", errors.Userf("no remote branches found")
	}

	wts, err := worktree.List(repoGit)
	if err != nil {
		return "", fmt.Errorf("failed to list worktrees: %w", err)
	}
	wtBranches := make(map[string]bool)
	for _, wt := range wts {
		if !wt.IsBare && !wt.Detached {
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
		return "", errors.Userf("all remote branches already have worktrees")
	}

	opts := []fzf.Option{
		fzf.WithReverse(),
		fzf.WithHeader("Select a branch to check out"),
	}
	if strings.TrimSpace(query) != "" {
		opts = append(opts, fzf.WithQuery(query))
	}

	selected, err := fzf.Run(available, opts...)
	if err != nil {
		return "", err
	}
	return selected, nil
}
