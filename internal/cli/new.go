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
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/ui"
	"github.com/iamrajjoshi/willow/internal/worktree"
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

// runPostCheckoutHook manually invokes the configured post-checkout hook from
// the new worktree. This is needed for bare repos because git resolves relative
// core.hooksPath against the bare repo dir (where hook files don't exist),
// so the hook never fires automatically.
func runPostCheckoutHook(hookPath, wtPath string, u *ui.UI) {
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
	cmd.Stdout = os.Stdout
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
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			tr := trace.New(flags.Trace)
			g := flags.NewGit()
			u := flags.NewUI()
			cdOnly := cmd.Bool("cd")
			existing := cmd.Bool("existing")

			// Resolve repo
			var bareDir string
			var err error
			done := tr.Start("resolve repo")
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

			done = tr.Start("load config")
			cfg := config.Load(bareDir)
			done()

			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			repoName := repoNameFromDir(bareDir)

			branch := cmd.StringArg("branch")

			// --pr flag: resolve PR number or URL to branch name
			if prRef := cmd.String("pr"); prRef != "" {
				done = tr.Start("resolve PR")
				prBranch, ok := resolvePRRef(prRef, bareDir)
				if !ok {
					return errs.Userf("failed to resolve PR: %s\n\nEnsure 'gh' is installed and you're authenticated", prRef)
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

			// PR URL auto-detection in branch arg
			if branch != "" && isPRURL(branch) {
				done = tr.Start("resolve PR branch")
				prBranch, ok := resolvePRRef(branch, bareDir)
				if !ok {
					return errs.Userf("failed to resolve branch from PR URL: %s\n\nEnsure 'gh' is installed and you're authenticated", branch)
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

			// Existing-branch picker: -e with no branch arg
			if existing && branch == "" {
				done = tr.Start("pick existing branch")
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
				// --- Existing branch path ---
				// No branch prefix for existing branches
				shouldFetch := *cfg.Defaults.Fetch && !cmd.Bool("no-fetch")
				if shouldFetch {
					done = tr.Start("git fetch branch")
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

				done = tr.Start("git worktree add")
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

				return finishWorktree(tr, cfg, g, u, wtPath, repoName, branch, "", cdOnly)
			}

			// --- New branch path ---
			// Apply branch prefix from config
			if cfg.BranchPrefix != "" && !strings.HasPrefix(branch, cfg.BranchPrefix+"/") {
				branch = cfg.BranchPrefix + "/" + branch
			}

			// Resolve base branch: flag → config → auto-detect
			done = tr.Start("resolve base branch")
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

			// Fetch latest from remote (only for remote bases)
			shouldFetch := *cfg.Defaults.Fetch && !cmd.Bool("no-fetch") && !localBase
			if shouldFetch {
				done = tr.Start("git fetch")
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

			done = tr.Start("git worktree add")
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

			// Record parent in stack for stacked PRs
			done = tr.Start("record stack parent")
			st := stack.Load(bareDir)
			st.SetParent(branch, baseBranch)
			if err := st.Save(bareDir); err != nil {
				u.Warn(fmt.Sprintf("Failed to save stack: %v", err))
			}
			done()

			return finishWorktree(tr, cfg, g, u, wtPath, repoName, branch, baseBranch, cdOnly)
		},
	}
}

func finishWorktree(tr *trace.Tracer, cfg *config.Config, g *git.Git, u *ui.UI, wtPath, repoName, branch, baseBranch string, cdOnly bool) error {
	done := tr.Start("post-checkout hook")
	runPostCheckoutHook(cfg.PostCheckoutHook, wtPath, u)
	done()

	done = tr.Start("auto setup remote")
	if *cfg.Defaults.AutoSetupRemote {
		wtGit := &git.Git{Dir: wtPath, Verbose: g.Verbose}
		if _, err := wtGit.Run("config", "--lock-timeout", "500", "--local", "push.autoSetupRemote", "true"); err != nil {
			u.Warn("Failed to set push.autoSetupRemote: " + err.Error())
		}
	}
	done()

	done = tr.Start("setup hooks")
	if len(cfg.Setup) > 0 && !cdOnly {
		u.Info("Running setup hooks...")
		if err := runHooks(cfg.Setup, wtPath, u); err != nil {
			return err
		}
	}
	done()

	done = tr.Start("pane commands")
	if !cdOnly {
		for _, w := range cfg.Validate() {
			u.Warn(w)
		}
	}
	if len(cfg.Tmux.Panes) > 0 && !cdOnly {
		for i, p := range cfg.Tmux.Panes {
			if p.Command != "" {
				if err := runHooks([]string{p.Command}, wtPath, u); err != nil {
					return errs.User(fmt.Errorf("pane %d command failed: %w", i, err))
				}
			}
		}
	}
	done()

	tr.Total()

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
func resolvePRRef(input, bareDir string) (string, bool) {
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return "", false
	}

	cmd := exec.Command(ghPath, "pr", "view", input, "--json", "headRefName", "-q", ".headRefName")
	cmd.Dir = bareDir
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}

	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "", false
	}
	return branch, true
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
