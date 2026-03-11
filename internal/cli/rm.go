package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func rmCmd() *cli.Command {
	return &cli.Command{
		Name:  "rm",
		Usage: "Remove a worktree and its branch",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "branch",
				UsageText: "[branch]",
			},
		},
		ShellComplete: completeWorktrees,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Skip safety checks",
			},
			&cli.BoolFlag{
				Name:  "keep-branch",
				Usage: "Remove worktree but keep the local branch",
			},
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Skip confirmation prompt",
			},
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target a willow-managed repo by name",
			},
			&cli.BoolFlag{
				Name:  "prune",
				Usage: "Run git worktree prune after removal",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			tr := trace.New(flags.Trace)
			g := flags.NewGit()
			u := flags.NewUI()

			done := tr.Start("resolve repo")
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

			done = tr.Start("list worktrees")
			repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
			worktrees, err := worktree.List(repoGit)
			if err != nil {
				return fmt.Errorf("failed to list worktrees: %w", err)
			}
			done()

			filtered := filterBareWorktrees(worktrees)

			done = tr.Start("resolve target")
			target := cmd.StringArg("branch")
			var wt *worktree.Worktree

			if target == "" {
				repoName := repoNameFromDir(bareDir)
				selectedPath, err := fzfPickWorktree(filtered, repoName)
				if err != nil {
					return err
				}
				if selectedPath == "" {
					return nil
				}
				for i := range filtered {
					if filtered[i].Path == selectedPath {
						wt = &filtered[i]
						break
					}
				}
				if wt == nil {
					return fmt.Errorf("selected worktree not found")
				}
			} else {
				wt, err = findWorktree(filtered, target)
				if err != nil {
					return err
				}
			}
			done()

			force := cmd.Bool("force")
			wtGit := &git.Git{Dir: wt.Path, Verbose: g.Verbose}

			if !force {
				done = tr.Start("check dirty")
				dirty, err := wtGit.IsDirty()
				if err != nil {
					return err
				}
				if dirty {
					u.Warn(fmt.Sprintf("Worktree %s has uncommitted changes", u.Bold(wt.Branch)))
				}
				done()

				done = tr.Start("check unpushed")
				unpushed, err := wtGit.HasUnpushedCommits()
				if err != nil {
					return err
				}
				if unpushed {
					u.Warn(fmt.Sprintf("Worktree %s has unpushed commits", u.Bold(wt.Branch)))
				}
				done()

				if (dirty || unpushed) && !cmd.Bool("yes") {
					if !confirm("Remove anyway?") {
						u.Info("Aborted.")
						return nil
					}
				} else if !cmd.Bool("yes") {
					if !confirm(fmt.Sprintf("Remove worktree %s?", wt.Branch)) {
						u.Info("Aborted.")
						return nil
					}
				}
			}

			done = tr.Start("load config")
			cfg := config.Load(bareDir)
			done()

			if len(cfg.Teardown) > 0 {
				done = tr.Start("teardown hooks")
				u.Info("Running teardown hooks...")
				if err := runHooks(cfg.Teardown, wt.Path, u); err != nil {
					return err
				}
				done()
			}

			// Move worktree to trash instead of blocking on rm -rf.
			// 1) Remove the git worktree admin dir so git no longer tracks it.
			// 2) Rename the worktree dir into ~/.willow/trash/<id> (instant on same FS).
			// 3) Spawn a detached rm -rf on the trash entry so the user isn't blocked.
			done = tr.Start("remove worktree")
			adminDir := filepath.Join(bareDir, "worktrees", filepath.Base(wt.Path))
			if err := os.RemoveAll(adminDir); err != nil {
				return fmt.Errorf("failed to remove worktree admin dir: %w", err)
			}

			trashDir := config.TrashDir()
			if err := os.MkdirAll(trashDir, 0o755); err != nil {
				return fmt.Errorf("failed to create trash dir: %w", err)
			}

			trashDest := filepath.Join(trashDir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), filepath.Base(wt.Path)))
			if err := os.Rename(wt.Path, trashDest); err != nil {
				// Rename can fail across filesystems; fall back to synchronous removal.
				if removeErr := os.RemoveAll(wt.Path); removeErr != nil {
					return fmt.Errorf("failed to remove worktree: %w", removeErr)
				}
			} else {
				bgRm := exec.Command("rm", "-rf", trashDest)
				bgRm.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
				_ = bgRm.Start()
			}
			done()

			done = tr.Start("git branch -D")
			if !cmd.Bool("keep-branch") {
				if _, err := repoGit.Run("branch", "-D", wt.Branch); err != nil {
					u.Warn(fmt.Sprintf("Failed to delete branch %s: %v", wt.Branch, err))
				}
			}
			done()

			done = tr.Start("cleanup status")
			repoName := repoNameFromDir(bareDir)
			wtDir := filepath.Base(wt.Path)
			claude.RemoveStatusDir(repoName, wtDir)
			done()

			u.Success(fmt.Sprintf("Removed worktree %s", u.Bold(wt.Branch)))

			if cmd.Bool("prune") {
				done = tr.Start("git worktree prune")
				u.Info("Pruning stale worktrees...")
				if _, err := repoGit.Run("worktree", "prune"); err != nil {
					u.Warn(fmt.Sprintf("Prune failed: %v", err))
				} else {
					u.Success("Pruned stale worktrees")
				}
				done()
			}

			tr.Total()
			return nil
		},
	}
}

func confirm(prompt string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}
