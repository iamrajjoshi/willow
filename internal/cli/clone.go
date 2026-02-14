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

			if _, err := os.Stat(bareDir); err == nil {
				return fmt.Errorf("repository %q already exists at %s", name, bareDir)
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

			// Bare clones don't set up remote tracking by default.
			// Configure the fetch refspec so `git fetch` populates refs/remotes/origin/*.
			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			if _, err := repoGit.Run("config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
				return fmt.Errorf("failed to configure fetch refs: %w", err)
			}

			u.Info("Fetching latest from origin...")
			if _, err := repoGit.Run("fetch", "origin"); err != nil {
				return fmt.Errorf("failed to fetch from origin: %w", err)
			}

			defaultBranch, err := detectDefaultBranch(repoGit)
			if err != nil {
				return fmt.Errorf("failed to detect default branch: %w", err)
			}

			// Create the initial worktree on the default branch
			wtPath := filepath.Join(worktreesDir, defaultBranch)
			u.Info(fmt.Sprintf("Creating worktree %s at %s...", u.Bold(defaultBranch), wtPath))
			if _, err := repoGit.Run("worktree", "add", wtPath, defaultBranch); err != nil {
				return fmt.Errorf("failed to create initial worktree: %w", err)
			}

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

