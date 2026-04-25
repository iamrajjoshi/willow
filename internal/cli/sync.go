package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errors"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/log"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/ui"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func syncCmd() *cli.Command {
	return &cli.Command{
		Name:  "sync",
		Usage: "Rebase stacked worktrees onto their parents",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "branch",
				UsageText: "[branch]",
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target a willow-managed repo by name",
			},
			&cli.BoolFlag{
				Name:  "no-fetch",
				Usage: "Skip fetching from remote",
			},
			&cli.BoolFlag{
				Name:  "abort",
				Usage: "Abort any in-progress rebases across stacked worktrees",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			tr := trace.FromContext(ctx)
			defer tr.Total()
			g := flags.NewGit()
			u := flags.NewUI()

			done := tr.StartCtx(ctx, "resolve repo")
			var bareDir string
			var err error
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

			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}

			done = tr.StartCtx(ctx, "load stack")
			st := stack.Load(bareDir)
			done()
			if st.IsEmpty() {
				u.Info("No stacked branches found. Use 'ww new <branch> -b <parent>' to create a stack.")
				return nil
			}

			done = tr.StartCtx(ctx, "list worktrees")
			wts, err := worktree.List(repoGit)
			if err != nil {
				return fmt.Errorf("failed to list worktrees: %w", err)
			}
			wtPaths := make(map[string]string) // branch → worktree path
			for _, wt := range wts {
				if !wt.IsBare {
					wtPaths[wt.Branch] = wt.Path
				}
			}
			if cmd.Bool("abort") {
				repoName := repoNameFromDir(bareDir)
				for _, branch := range st.TopoSort() {
					if _, ok := wtPaths[branch]; ok {
						continue
					}
					dirName := strings.ReplaceAll(branch, "/", "-")
					candidate := filepath.Join(config.WorktreesDir(), repoName, dirName)
					if info, err := os.Stat(candidate); err == nil && info.IsDir() {
						wtPaths[branch] = candidate
					}
				}
			}
			done()

			if cmd.Bool("abort") {
				return syncAbort(st, wtPaths, g.Verbose, u)
			}

			var branches []string
			if targetBranch := cmd.StringArg("branch"); targetBranch != "" {
				if !st.IsTracked(targetBranch) {
					return errors.Userf("branch %q is not in the stack", targetBranch)
				}
				branches = st.SubtreeSort(targetBranch)
			} else {
				branches = st.TopoSort()
			}

			if len(branches) == 0 {
				u.Info("Nothing to sync.")
				return nil
			}

			if !cmd.Bool("no-fetch") {
				done = tr.StartCtx(ctx, "git fetch")
				if err := u.Spin("Fetching origin", func() error {
					_, err := repoGit.Run("fetch", "origin")
					return err
				}); err != nil {
					u.Warn(fmt.Sprintf("fetch failed: %v (continuing anyway)", err))
				}
				done()
			}

			u.Info(fmt.Sprintf("\nSyncing %d stacked worktree(s):\n", len(branches)))

			conflicted := make(map[string]bool)
			synced := 0
			skipped := 0

			for _, branch := range branches {
				parent := st.Parent(branch)

				if isAncestorConflicted(st, branch, conflicted) {
					u.Info(fmt.Sprintf("  %s → %s", parent, branch))
					u.Info(fmt.Sprintf("    %s Skipped (ancestor has conflict)", u.Dim("⊘")))
					skipped++
					continue
				}

				wtPath, hasWorktree := wtPaths[branch]
				if !hasWorktree {
					u.Info(fmt.Sprintf("  %s → %s", parent, branch))
					u.Info(fmt.Sprintf("    %s Skipped (no worktree)", u.Dim("⊘")))
					skipped++
					continue
				}

				wtGit := &git.Git{Dir: wtPath, Verbose: g.Verbose}

				dirty, err := wtGit.IsDirty()
				if err != nil {
					u.Info(fmt.Sprintf("  %s → %s", parent, branch))
					u.Warn(fmt.Sprintf("    ⚠ Skipped (failed to check status: %v)", err))
					skipped++
					continue
				}
				if dirty {
					u.Info(fmt.Sprintf("  %s → %s", parent, branch))
					u.Warn(fmt.Sprintf("    ⚠ Skipped (uncommitted changes)"))
					skipped++
					continue
				}

				if wtGit.IsRebaseInProgress() {
					u.Info(fmt.Sprintf("  %s → %s", parent, branch))
					u.Warn(fmt.Sprintf("    ⚠ Rebase in progress — resolve manually"))
					conflicted[branch] = true
					continue
				}

				rebaseOnto := parent
				if !st.IsTracked(parent) {
					rebaseOnto = "origin/" + parent
				}

				u.Info(fmt.Sprintf("  %s → %s", parent, u.Bold(branch)))

				if err := wtGit.Rebase(rebaseOnto); err != nil {
					conflicted[branch] = true
					u.Warn(fmt.Sprintf("    ✗ Conflict — resolve in %s", wtPath))
					u.Info(fmt.Sprintf("      cd %s && git rebase --continue", wtPath))
					continue
				}

				ahead, aheadErr := wtGit.CommitsAhead(rebaseOnto)
				if aheadErr != nil {
					u.Info(fmt.Sprintf("    %s Rebased onto %s", u.Green("✔"), rebaseOnto))
				} else {
					u.Info(fmt.Sprintf("    %s Rebased onto %s (%d commits ahead)", u.Green("✔"), rebaseOnto, ahead))
				}
				meta := map[string]string{"parent": parent}
				if aheadErr == nil {
					meta["ahead"] = fmt.Sprintf("%d", ahead)
				}
				_ = log.Append(log.Event{
					Action:   "sync",
					Repo:     repoNameFromDir(bareDir),
					Branch:   branch,
					Metadata: meta,
				})
				synced++
			}

			fmt.Println()
			if len(conflicted) > 0 {
				u.Warn(fmt.Sprintf("%d synced, %d conflicted, %d skipped", synced, len(conflicted), skipped))
				u.Info("After resolving conflicts, run 'ww sync' again.")
			} else {
				u.Success(fmt.Sprintf("All %d worktree(s) synced.", synced))
			}
			return nil
		},
	}
}

func syncAbort(st *stack.Stack, wtPaths map[string]string, verbose bool, u *ui.UI) error {
	aborted := 0
	for _, branch := range st.TopoSort() {
		wtPath, ok := wtPaths[branch]
		if !ok {
			continue
		}
		wtGit := &git.Git{Dir: wtPath, Verbose: verbose}
		if wtGit.IsRebaseInProgress() {
			u.Info(fmt.Sprintf("Aborting rebase in %s...", branch))
			if err := wtGit.RebaseAbort(); err != nil {
				u.Warn(fmt.Sprintf("  Failed to abort rebase in %s: %v", branch, err))
			} else {
				aborted++
			}
		}
	}
	if aborted == 0 {
		u.Info("No rebases in progress.")
	} else {
		u.Success(fmt.Sprintf("Aborted %d rebase(s).", aborted))
	}
	return nil
}

// isAncestorConflicted checks if any ancestor of branch in the stack has a conflict.
func isAncestorConflicted(st *stack.Stack, branch string, conflicted map[string]bool) bool {
	parent := st.Parent(branch)
	for parent != "" && st.IsTracked(parent) {
		if conflicted[parent] {
			return true
		}
		parent = st.Parent(parent)
	}
	return false
}
