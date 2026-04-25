package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errors"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/parallel"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

// findWorktree matches a target string against worktrees by exact branch name,
// directory name, or substring match on the branch.
func findWorktree(worktrees []worktree.Worktree, target string) (*worktree.Worktree, error) {
	for i := range worktrees {
		if worktrees[i].Branch == target || worktrees[i].MatchName() == target {
			return &worktrees[i], nil
		}
	}

	var matches []worktree.Worktree
	for _, wt := range worktrees {
		if strings.Contains(wt.MatchName(), target) || strings.Contains(wt.DisplayName(), target) || strings.HasSuffix(wt.Path, "/"+target) {
			matches = append(matches, wt)
		}
	}

	switch len(matches) {
	case 0:
		return nil, errors.Userf("no worktree found matching %q", target)
	case 1:
		return &matches[0], nil
	default:
		lines := fmt.Sprintf("ambiguous match %q, could be:\n", target)
		for _, wt := range matches {
			lines += fmt.Sprintf("  %s  %s\n", wt.DisplayName(), wt.Path)
		}
		return nil, errors.User(fmt.Errorf("%s", strings.TrimRight(lines, "\n")))
	}
}

// filterBareWorktrees returns only non-bare worktrees.
func filterBareWorktrees(worktrees []worktree.Worktree) []worktree.Worktree {
	var filtered []worktree.Worktree
	for _, wt := range worktrees {
		if !wt.IsBare {
			filtered = append(filtered, wt)
		}
	}
	return filtered
}

func worktreeDirName(name string) string {
	return strings.ReplaceAll(name, "/", "-")
}

type repoInfo struct {
	Name    string
	BareDir string
}

type repoWorktree struct {
	Repo     repoInfo
	Worktree worktree.Worktree
}

// resolveRepos returns bare dirs to operate on.
// Priority: -r flag > current repo > all repos.
func resolveRepos(g *git.Git, repoFlag string) ([]repoInfo, error) {
	if repoFlag != "" {
		bareDir, err := config.ResolveRepo(repoFlag)
		if err != nil {
			return nil, err
		}
		return []repoInfo{{Name: repoFlag, BareDir: bareDir}}, nil
	}

	bareDir, err := requireWillowRepo(g)
	if err == nil {
		return []repoInfo{{Name: repoNameFromDir(bareDir), BareDir: bareDir}}, nil
	}

	repos, err := config.ListRepos()
	if err != nil {
		return nil, fmt.Errorf("failed to list repos: %w", err)
	}
	if len(repos) == 0 {
		return nil, errors.Userf("no willow-managed repos found\n\nUse 'ww clone <url>' to get started.")
	}

	var result []repoInfo
	for _, r := range repos {
		bd, err := config.ResolveRepo(r)
		if err != nil {
			continue
		}
		result = append(result, repoInfo{Name: r, BareDir: bd})
	}
	return result, nil
}

func collectAllWorktrees(repos []repoInfo, verbose bool) []repoWorktree {
	results := parallel.Map(repos, func(_ int, r repoInfo) []repoWorktree {
		repoGit := &git.Git{Dir: r.BareDir, Verbose: verbose}
		wts, err := worktree.List(repoGit)
		if err != nil {
			return nil
		}
		var repoWts []repoWorktree
		for _, wt := range filterBareWorktrees(wts) {
			repoWts = append(repoWts, repoWorktree{Repo: r, Worktree: wt})
		}
		return repoWts
	})

	var all []repoWorktree
	for _, repoWts := range results {
		all = append(all, repoWts...)
	}
	return all
}

func findCrossRepoWorktree(rwts []repoWorktree, target string) (*repoWorktree, error) {
	for i := range rwts {
		if rwts[i].Worktree.Branch == target || rwts[i].Worktree.MatchName() == target {
			return &rwts[i], nil
		}
	}

	var matches []repoWorktree
	for _, rwt := range rwts {
		if strings.Contains(rwt.Worktree.MatchName(), target) || strings.Contains(rwt.Worktree.DisplayName(), target) || strings.HasSuffix(rwt.Worktree.Path, "/"+target) {
			matches = append(matches, rwt)
		}
	}

	switch len(matches) {
	case 0:
		return nil, errors.Userf("no worktree found matching %q", target)
	case 1:
		return &matches[0], nil
	default:
		lines := fmt.Sprintf("ambiguous match %q, could be:\n", target)
		for _, rwt := range matches {
			lines += fmt.Sprintf("  %s/%s  %s\n", rwt.Repo.Name, rwt.Worktree.DisplayName(), rwt.Worktree.Path)
		}
		return nil, errors.User(fmt.Errorf("%s", strings.TrimRight(lines, "\n")))
	}
}

// repoWorktreeByPath looks up the repoWorktree for a given path.
func repoWorktreeByPath(rwts []repoWorktree, path string) *repoWorktree {
	for i := range rwts {
		if rwts[i].Worktree.Path == path {
			return &rwts[i]
		}
	}
	return nil
}

// completeWorktreesCrossRepo provides shell completion across all repos.
func completeWorktreesCrossRepo(ctx context.Context, cmd *cli.Command) {
	if args := cmd.Args().Slice(); len(args) > 0 && strings.HasPrefix(args[len(args)-1], "-") {
		cli.DefaultCompleteWithFlags(ctx, cmd)
		return
	}

	repos, err := config.ListRepos()
	if err != nil {
		return
	}
	w := cmd.Root().Writer
	for _, r := range repos {
		bd, err := config.ResolveRepo(r)
		if err != nil {
			continue
		}
		repoGit := &git.Git{Dir: bd}
		wts, err := worktree.List(repoGit)
		if err != nil {
			continue
		}
		for _, wt := range wts {
			if !wt.IsBare {
				fmt.Fprintln(w, wt.MatchName())
			}
		}
	}
}

// completeWorktreesWithFlag provides shell completion respecting -r flag, with cross-repo fallback.
func completeWorktreesWithFlag(ctx context.Context, cmd *cli.Command) {
	if args := cmd.Args().Slice(); len(args) > 0 && strings.HasPrefix(args[len(args)-1], "-") {
		cli.DefaultCompleteWithFlags(ctx, cmd)
		return
	}

	if repoFlag := cmd.String("repo"); repoFlag != "" {
		bareDir, err := config.ResolveRepo(repoFlag)
		if err != nil {
			return
		}
		repoGit := &git.Git{Dir: bareDir}
		wts, err := worktree.List(repoGit)
		if err != nil {
			return
		}
		w := cmd.Root().Writer
		for _, wt := range wts {
			if !wt.IsBare {
				fmt.Fprintln(w, wt.MatchName())
			}
		}
		return
	}

	g := &git.Git{}
	bareDir, err := g.BareRepoDir()
	if err == nil && config.IsWillowRepo(bareDir) {
		repoGit := &git.Git{Dir: bareDir}
		wts, err := worktree.List(repoGit)
		if err != nil {
			return
		}
		w := cmd.Root().Writer
		for _, wt := range wts {
			if !wt.IsBare {
				fmt.Fprintln(w, wt.MatchName())
			}
		}
		return
	}
	if bareDir, ok := resolveRepoFromCwd(); ok {
		repoGit := &git.Git{Dir: bareDir}
		wts, err := worktree.List(repoGit)
		if err != nil {
			return
		}
		w := cmd.Root().Writer
		for _, wt := range wts {
			if !wt.IsBare {
				fmt.Fprintln(w, wt.MatchName())
			}
		}
		return
	}

	completeWorktreesCrossRepo(ctx, cmd)
}
