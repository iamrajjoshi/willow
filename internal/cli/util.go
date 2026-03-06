package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

// completeWorktrees provides shell completion for commands that take a branch argument.
func completeWorktrees(ctx context.Context, cmd *cli.Command) {
	if args := cmd.Args().Slice(); len(args) > 0 && strings.HasPrefix(args[len(args)-1], "-") {
		cli.DefaultCompleteWithFlags(ctx, cmd)
		return
	}

	g := &git.Git{}
	bareDir, err := g.BareRepoDir()
	if err != nil {
		if dir, ok := resolveRepoFromCwd(); ok {
			bareDir = dir
		} else {
			return
		}
	}
	repoGit := &git.Git{Dir: bareDir}
	wts, err := worktree.List(repoGit)
	if err != nil {
		return
	}
	w := cmd.Root().Writer
	for _, wt := range wts {
		if !wt.IsBare {
			fmt.Fprintln(w, wt.Branch)
		}
	}
}

// findWorktree matches a target string against worktrees by exact branch name,
// directory name, or substring match on the branch.
func findWorktree(worktrees []worktree.Worktree, target string) (*worktree.Worktree, error) {
	for i := range worktrees {
		if worktrees[i].Branch == target {
			return &worktrees[i], nil
		}
	}

	var matches []worktree.Worktree
	for _, wt := range worktrees {
		if strings.Contains(wt.Branch, target) || strings.HasSuffix(wt.Path, "/"+target) {
			matches = append(matches, wt)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no worktree found matching %q", target)
	case 1:
		return &matches[0], nil
	default:
		lines := fmt.Sprintf("ambiguous match %q, could be:\n", target)
		for _, wt := range matches {
			lines += fmt.Sprintf("  %s  %s\n", wt.Branch, wt.Path)
		}
		return nil, fmt.Errorf("%s", strings.TrimRight(lines, "\n"))
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
