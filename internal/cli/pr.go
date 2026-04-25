package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errors"
	"github.com/iamrajjoshi/willow/internal/gh"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/log"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func prCmd() *cli.Command {
	return &cli.Command{
		Name:  "pr",
		Usage: "GitHub pull request workflows",
		Commands: []*cli.Command{
			prCreateCmd(),
		},
	}
}

func prCreateCmd() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "Create a GitHub pull request for the current branch or stack",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "draft",
				Usage: "Create draft pull requests",
			},
			&cli.BoolFlag{
				Name:  "stack",
				Usage: "Create missing pull requests for the current branch's ancestor stack",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.pr.create")()
			flags := parseFlags(cmd)
			g := flags.NewGit()
			u := flags.NewUI()

			wtPath, bareDir, err := requireWillowWorktree(g)
			if err != nil {
				return err
			}
			if err := gh.EnsureCLI("PR creation"); err != nil {
				return err
			}

			cfg := config.Load(bareDir)
			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			currentGit := &git.Git{Dir: wtPath, Verbose: g.Verbose}

			currentBranch, err := currentBranchName(currentGit)
			if err != nil {
				return err
			}
			if err := ensureCleanWorktree(currentGit, currentBranch); err != nil {
				return err
			}

			st := stack.Load(bareDir)
			branches := prCreateBranches(st, currentBranch, cmd.Bool("stack"))
			wtPaths, err := worktreePathsByBranch(repoGit)
			if err != nil {
				return fmt.Errorf("failed to list worktrees: %w", err)
			}

			if len(branches) > 1 {
				u.Info(fmt.Sprintf("Creating PRs for %d branches:\n", len(branches)))
			}

			created := 0
			existing := 0
			draft := cmd.Bool("draft")
			for _, branch := range branches {
				base := prBaseBranch(st, repoGit, cfg, branch)
				u.Info(fmt.Sprintf("  %s → %s", u.Bold(branch), base))

				branchPath := wtPaths[branch]
				if branchPath != "" {
					branchGit := &git.Git{Dir: branchPath, Verbose: g.Verbose}
					if err := ensureCleanWorktree(branchGit, branch); err != nil {
						return err
					}
				}

				pushed, err := pushBranchIfNeeded(repoGit, branchPath, branch)
				if err != nil {
					return err
				}
				if pushed {
					u.Info(fmt.Sprintf("    %s Pushed origin/%s", u.Dim("↑"), branch))
				}

				headOID, err := branchHeadOID(repoGit, branchPath, branch)
				if err != nil {
					return err
				}

				lookupDir := wtPath
				if branchPath != "" {
					lookupDir = branchPath
				}
				existingPR, err := gh.FindOpenPR(lookupDir, branch, headOID)
				if err != nil {
					return err
				}
				if existingPR != nil {
					if existingPR.URL != "" {
						u.Info(fmt.Sprintf("    %s Existing PR %s", u.Dim("↺"), existingPR.URL))
					} else {
						u.Info(fmt.Sprintf("    %s Existing PR #%d", u.Dim("↺"), existingPR.Number))
					}
					existing++
					continue
				}

				url, err := gh.CreatePR(wtPath, base, branch, draft)
				if err != nil {
					return err
				}

				meta := map[string]string{
					"base":  base,
					"draft": fmt.Sprintf("%t", draft),
				}
				if url != "" {
					meta["url"] = url
				}
				_ = log.Append(log.Event{
					Action:   "pr_create",
					Repo:     repoNameFromDir(bareDir),
					Branch:   branch,
					Metadata: meta,
				})

				if url != "" {
					u.Success(fmt.Sprintf("Created PR for %s: %s", branch, url))
				} else {
					u.Success(fmt.Sprintf("Created PR for %s", branch))
				}
				created++
			}

			if len(branches) > 1 {
				u.Info("")
				u.Success(fmt.Sprintf("%d PR(s) created, %d already existed", created, existing))
			}

			return nil
		},
	}
}

func requireWillowWorktree(g *git.Git) (string, string, error) {
	wtPath, err := g.WorktreeRoot()
	if err != nil {
		return "", "", errors.Userf("not inside a willow-managed worktree\n\nRun this command from a willow-managed worktree.")
	}

	bareDir, err := g.BareRepoDir()
	if err != nil || !config.IsWillowRepo(bareDir) {
		return "", "", errors.Userf("not inside a willow-managed worktree\n\nRun this command from a willow-managed worktree.")
	}

	return wtPath, bareDir, nil
}

func currentBranchName(g *git.Git) (string, error) {
	branch, err := g.Run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to resolve current branch: %w", err)
	}

	branch = strings.TrimSpace(branch)
	if branch == "" || branch == "HEAD" || branch == "(detached)" {
		return "", errors.Userf("current worktree is in detached HEAD state\n\nCheck out a branch before creating a PR.")
	}

	return branch, nil
}

func ensureCleanWorktree(g *git.Git, branch string) error {
	dirty, err := g.IsDirty()
	if err != nil {
		return fmt.Errorf("failed to check worktree %q for uncommitted changes: %w", branch, err)
	}
	if dirty {
		return errors.Userf("worktree %q has uncommitted changes\n\nCommit or stash changes before creating a PR.", branch)
	}
	return nil
}

func prCreateBranches(st *stack.Stack, currentBranch string, includeAncestors bool) []string {
	if !includeAncestors || !st.IsTracked(currentBranch) {
		return []string{currentBranch}
	}

	chain := []string{currentBranch}
	for parent := st.Parent(currentBranch); parent != "" && st.IsTracked(parent); parent = st.Parent(parent) {
		chain = append(chain, parent)
	}

	for left, right := 0, len(chain)-1; left < right; left, right = left+1, right-1 {
		chain[left], chain[right] = chain[right], chain[left]
	}
	return chain
}

func prBaseBranch(st *stack.Stack, repoGit *git.Git, cfg *config.Config, branch string) string {
	if st.IsTracked(branch) {
		if parent := st.Parent(branch); parent != "" {
			return parent
		}
	}
	return repoGit.ResolveBaseBranch(cfg.BaseBranch)
}

func worktreePathsByBranch(repoGit *git.Git) (map[string]string, error) {
	wts, err := worktree.List(repoGit)
	if err != nil {
		return nil, err
	}

	paths := make(map[string]string, len(wts))
	for _, wt := range filterBareWorktrees(wts) {
		if wt.Detached {
			continue
		}
		paths[wt.Branch] = wt.Path
	}
	return paths, nil
}

func branchHeadOID(repoGit *git.Git, wtPath, branch string) (string, error) {
	if wtPath != "" {
		out, err := (&git.Git{Dir: wtPath, Verbose: repoGit.Verbose}).Run("rev-parse", "HEAD")
		if err != nil {
			return "", fmt.Errorf("failed to resolve HEAD for branch %q: %w", branch, err)
		}
		return strings.TrimSpace(out), nil
	}

	out, err := repoGit.Run("rev-parse", branch)
	if err != nil {
		return "", fmt.Errorf("failed to resolve branch %q: %w", branch, err)
	}
	return strings.TrimSpace(out), nil
}

func pushBranchIfNeeded(repoGit *git.Git, wtPath, branch string) (bool, error) {
	needsPush, err := branchNeedsPush(repoGit, branch)
	if err != nil {
		return false, err
	}
	if !needsPush {
		return false, nil
	}

	gitRunner := repoGit
	args := []string{"push", "origin", branch}
	if wtPath != "" {
		gitRunner = &git.Git{Dir: wtPath, Verbose: repoGit.Verbose}
		args = []string{"push", "-u", "origin", branch}
	}

	if _, err := gitRunner.Run(args...); err != nil {
		return false, fmt.Errorf("failed to push branch %q: %w", branch, err)
	}
	return true, nil
}

func branchNeedsPush(repoGit *git.Git, branch string) (bool, error) {
	if !repoGit.RemoteBranchExists(branch) {
		return true, nil
	}

	out, err := repoGit.Run("rev-list", "--left-right", "--count", "origin/"+branch+"..."+branch)
	if err != nil {
		return false, fmt.Errorf("failed to compare branch %q to origin: %w", branch, err)
	}

	trimmed := strings.TrimSpace(out)
	var behind, ahead int
	if _, err := fmt.Sscanf(trimmed, "%d\t%d", &behind, &ahead); err != nil {
		if _, err := fmt.Sscanf(trimmed, "%d %d", &behind, &ahead); err != nil {
			return false, fmt.Errorf("failed to parse ahead/behind count %q for branch %q: %w", out, branch, err)
		}
	}

	if behind > 0 {
		if ahead > 0 {
			return false, errors.Userf("branch %q has diverged from origin/%s\n\nReconcile the branch before creating a PR.", branch, branch)
		}
		return false, errors.Userf("branch %q is behind origin/%s\n\nFast-forward or reset the branch before creating a PR.", branch, branch)
	}
	return ahead > 0, nil
}
