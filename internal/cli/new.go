package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/trace"
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
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			tr := trace.New(flags.Trace)
			g := flags.NewGit()
			u := flags.NewUI()
			cdOnly := cmd.Bool("cd")

			branch := cmd.StringArg("branch")
			if branch == "" {
				return fmt.Errorf("branch name is required\n\nUsage: ww new <branch> [flags]")
			}

			var bareDir string
			var err error
			t := time.Now()
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
			tr.Step("resolve repo", t)

			t = time.Now()
			cfg := config.Load(bareDir)
			tr.Step("load config", t)

			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			repoName := repoNameFromDir(bareDir)

			// Apply branch prefix from config
			if cfg.BranchPrefix != "" && !strings.HasPrefix(branch, cfg.BranchPrefix+"/") {
				branch = cfg.BranchPrefix + "/" + branch
			}

			// Resolve base branch: flag → config → auto-detect
			t = time.Now()
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
			tr.Step("resolve base branch", t)

			// Fetch latest from remote (config default, --no-fetch overrides)
			shouldFetch := *cfg.Defaults.Fetch && !cmd.Bool("no-fetch")
			if shouldFetch {
				if !cdOnly {
					u.Info(fmt.Sprintf("Fetching %s from origin...", u.Bold(baseBranch)))
				}
				t = time.Now()
				if _, err := repoGit.Run("fetch", "origin", baseBranch); err != nil {
					return fmt.Errorf("failed to fetch origin/%s: %w", baseBranch, err)
				}
				tr.Step("git fetch", t)
			}

			dirName := strings.ReplaceAll(branch, "/", "-")
			wtPath := filepath.Join(config.WorktreesDir(), repoName, dirName)

			t = time.Now()
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
			tr.Step("git worktree add", t)

			t = time.Now()
			runPostCheckoutHook(cfg.PostCheckoutHook, wtPath, u)
			tr.Step("post-checkout hook", t)

			t = time.Now()
			if *cfg.Defaults.AutoSetupRemote {
				wtGit := &git.Git{Dir: wtPath, Verbose: g.Verbose}
				if _, err := wtGit.Run("config", "--local", "push.autoSetupRemote", "true"); err != nil {
					return fmt.Errorf("failed to configure push.autoSetupRemote: %w", err)
				}
			}
			tr.Step("auto setup remote", t)

			// Run setup hooks
			t = time.Now()
			if len(cfg.Setup) > 0 && !cdOnly {
				u.Info("Running setup hooks...")
				if err := runHooks(cfg.Setup, wtPath, u); err != nil {
					return err
				}
			}
			tr.Step("setup hooks", t)

			tr.Total()

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
