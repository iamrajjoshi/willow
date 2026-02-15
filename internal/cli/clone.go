package cli

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/urfave/cli/v3"
)

func cloneCmd() *cli.Command {
	return &cli.Command{
		Name:  "clone",
		Usage: "Clone a repo for willow (bare clone)",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "url",
				UsageText: "<repo-url>",
			},
			&cli.StringArg{
				Name:      "name",
				UsageText: "[name]",
			},
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Remove existing repo and re-clone from scratch",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			g := flags.NewGit()
			u := flags.NewUI()

			url := cmd.StringArg("url")
			if url == "" {
				return fmt.Errorf("repository URL is required\n\nUsage: ww clone <repo-url> [name]")
			}

			name := cmd.StringArg("name")
			if name == "" {
				name = repoNameFromURL(url)
			}

			bareDir := filepath.Join(config.ReposDir(), name+".git")
			worktreesDir := filepath.Join(config.WorktreesDir(), name)
			force := cmd.Bool("force")

			if _, err := os.Stat(bareDir); err == nil {
				if !force {
					return fmt.Errorf("repository %q already exists at %s\n\nRun with --force to remove it and re-clone", name, bareDir)
				}
				u.Info(fmt.Sprintf("Removing existing repo %s...", u.Bold(name)))
				if err := os.RemoveAll(bareDir); err != nil {
					return fmt.Errorf("failed to remove bare repo: %w", err)
				}
				if err := os.RemoveAll(worktreesDir); err != nil {
					return fmt.Errorf("failed to remove worktrees: %w", err)
				}
			}

			if err := os.MkdirAll(config.ReposDir(), 0o755); err != nil {
				return fmt.Errorf("failed to create repos directory: %w", err)
			}
			if err := os.MkdirAll(worktreesDir, 0o755); err != nil {
				return fmt.Errorf("failed to create worktrees directory: %w", err)
			}

			// Bare clone
			u.Info(fmt.Sprintf("Cloning %s into %s...", url, u.Bold(bareDir)))
			if _, err := g.Run("clone", "--bare", url, bareDir); err != nil {
				return fmt.Errorf("failed to clone repository: %w", err)
			}

			// If anything below fails, clean up the partial clone
			cleanup := func() {
				os.RemoveAll(bareDir)
				os.RemoveAll(worktreesDir)
			}

			// Bare clones don't set up remote tracking by default.
			// Configure the fetch refspec so `git fetch` populates refs/remotes/origin/*.
			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			if _, err := repoGit.Run("config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
				cleanup()
				return fmt.Errorf("failed to configure fetch refs: %w", err)
			}

			u.Info("Fetching latest from origin...")
			if _, err := repoGit.Run("fetch", "origin"); err != nil {
				cleanup()
				return fmt.Errorf("failed to fetch from origin: %w", err)
			}

			defaultBranch, err := repoGit.DefaultBranch()
			if err != nil {
				cleanup()
				return fmt.Errorf("failed to detect default branch: %w", err)
			}

			// Create the initial worktree on the default branch
			wtPath := filepath.Join(worktreesDir, defaultBranch)
			u.Info(fmt.Sprintf("Creating worktree %s at %s...", u.Bold(defaultBranch), wtPath))
			if _, err := repoGit.Run("worktree", "add", wtPath, defaultBranch); err != nil {
				cleanup()
				return fmt.Errorf("failed to create initial worktree: %w", err)
			}

			cfg := config.Load(bareDir, wtPath)
			runPostCheckoutHook(cfg.PostCheckoutHook, wtPath, u)

			u.Success(fmt.Sprintf("Cloned %s", u.Bold(name)))
			u.Info(fmt.Sprintf("  bare repo:  %s", u.Dim(bareDir)))
			u.Info(fmt.Sprintf("  worktree:   %s", u.Dim(wtPath)))
			return nil
		},
	}
}

// repoNameFromURL extracts the repository name from a git URL.
// Handles both SSH (git@github.com:org/repo.git) and HTTPS (https://github.com/org/repo.git).
func repoNameFromURL(url string) string {
	if i := strings.LastIndex(url, ":"); i != -1 && !strings.Contains(url, "://") {
		url = url[i+1:]
	}
	name := path.Base(url)
	return strings.TrimSuffix(name, ".git")
}

