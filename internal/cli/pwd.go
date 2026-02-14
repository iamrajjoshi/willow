package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func pwdCmd() *cli.Command {
	return &cli.Command{
		Name:  "pwd",
		Usage: "Print worktree path",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "branch",
				UsageText: "<branch-or-name>",
			},
		},
		ShellComplete: completeWorktrees,
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			g := flags.NewGit()

			target := cmd.StringArg("branch")
			if target == "" {
				return fmt.Errorf("branch or worktree name is required\n\nUsage: ww pwd <branch-or-name>")
			}

			bareDir, err := g.BareRepoDir()
			if err != nil {
				return err
			}

			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			worktrees, err := worktree.List(repoGit)
			if err != nil {
				return fmt.Errorf("failed to list worktrees: %w", err)
			}

			wt, err := findWorktree(worktrees, target)
			if err != nil {
				return err
			}

			fmt.Println(wt.Path)
			return nil
		},
	}
}

// completeWorktrees provides shell completion for commands that take a branch-or-name argument.
// When the user is typing a flag, it delegates to the default flag completer.
func completeWorktrees(ctx context.Context, cmd *cli.Command) {
	if args := cmd.Args().Slice(); len(args) > 0 && strings.HasPrefix(args[len(args)-1], "-") {
		cli.DefaultCompleteWithFlags(ctx, cmd)
		return
	}

	g := &git.Git{}
	bareDir, err := g.BareRepoDir()
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
			fmt.Fprintln(w, wt.Branch)
		}
	}
}

// findWorktree matches a target string against worktrees by exact branch name,
// directory name, or substring match on the branch.
func findWorktree(worktrees []worktree.Worktree, target string) (*worktree.Worktree, error) {
	// Exact branch match
	for i := range worktrees {
		if worktrees[i].Branch == target {
			return &worktrees[i], nil
		}
	}

	// Substring match on branch name or directory name
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
