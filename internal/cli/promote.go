package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errors"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/log"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/tmux"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func promoteCmd() *cli.Command {
	return &cli.Command{
		Name:  "promote",
		Usage: "Promote a detached worktree to a branch",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "target",
				UsageText: "[worktree]",
			},
			&cli.StringArg{
				Name:      "branch",
				UsageText: "<branch>",
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target a willow-managed repo by name",
			},
			&cli.StringFlag{
				Name:    "base",
				Aliases: []string{"b"},
				Usage:   "Record a stack parent for the promoted branch",
			},
			&cli.BoolFlag{
				Name:  "cd",
				Usage: "Print the final worktree path to stdout",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			tr := trace.FromContext(ctx)
			defer tr.Total()
			g := flags.NewGit()
			u := flags.NewUI()
			cdOnly := cmd.Bool("cd")
			if cdOnly {
				u.Out = os.Stderr
			}

			target, branch, explicitBranch := promotionArgs(g, cmd.StringArg("target"), cmd.StringArg("branch"))
			if branch == "" {
				return errors.Userf("branch name is required\n\nUsage: ww promote [worktree] <branch>")
			}

			done := tr.StartCtx(ctx, "resolve promotion target")
			rwt, err := resolvePromotionTarget(g, cmd.String("repo"), target)
			if err != nil {
				return err
			}
			done()

			if !rwt.Worktree.Detached {
				return errors.Userf("worktree %q is already on branch %q", rwt.Worktree.MatchName(), rwt.Worktree.Branch)
			}
			generatedDetached := isGeneratedDetachedDirName(rwt.Worktree.DirName())
			if generatedDetached && !explicitBranch {
				return errors.Userf("branch name is required for generated detached worktree %q\n\nUsage: ww promote %s <branch>", rwt.Worktree.MatchName(), rwt.Worktree.MatchName())
			}

			done = tr.StartCtx(ctx, "load config")
			cfg := config.Load(rwt.Repo.BareDir)
			done()

			if cfg.BranchPrefix != "" && !strings.HasPrefix(branch, cfg.BranchPrefix+"/") {
				branch = cfg.BranchPrefix + "/" + branch
			}

			repoGit := &git.Git{Dir: rwt.Repo.BareDir, Verbose: g.Verbose}
			if repoGit.LocalBranchExists(branch) {
				return errors.Userf("branch %q already exists", branch)
			}

			finalPath := rwt.Worktree.Path
			oldDir := rwt.Worktree.DirName()
			newDir := oldDir
			if generatedDetached {
				newDir = worktreeDirName(branch)
				if err := checkGeneratedPromotionCollisions(rwt.Repo.Name, oldDir, newDir, finalPath, filepath.Join(filepath.Dir(finalPath), newDir)); err != nil {
					return err
				}
				finalPath = filepath.Join(filepath.Dir(rwt.Worktree.Path), newDir)
			}

			done = tr.StartCtx(ctx, "git checkout branch")
			wtGit := &git.Git{Dir: rwt.Worktree.Path, Verbose: g.Verbose}
			if _, err := wtGit.Run("checkout", "-b", branch); err != nil {
				return fmt.Errorf("failed to promote detached worktree: %w", err)
			}
			done()

			done = tr.StartCtx(ctx, "auto setup remote")
			if *cfg.Defaults.AutoSetupRemote {
				if _, err := wtGit.Run("config", "--local", "push.autoSetupRemote", "true"); err != nil {
					u.Warn("Failed to set push.autoSetupRemote: " + err.Error())
				}
			}
			done()

			if generatedDetached && comparablePath(rwt.Worktree.Path) != comparablePath(finalPath) {
				done = tr.StartCtx(ctx, "git worktree move promoted")
				if _, err := repoGit.Run("worktree", "move", rwt.Worktree.Path, finalPath); err != nil {
					return fmt.Errorf("failed to move promoted worktree: %w", err)
				}
				done()

				done = tr.StartCtx(ctx, "move promoted status dir")
				if err := claude.MoveStatusDir(rwt.Repo.Name, oldDir, newDir); err != nil {
					return fmt.Errorf("failed to move status directory: %w", err)
				}
				done()

				done = tr.StartCtx(ctx, "rename promoted tmux session")
				oldSession := tmux.SessionNameForWorktree(rwt.Repo.Name, oldDir)
				newSession := tmux.SessionNameForWorktree(rwt.Repo.Name, newDir)
				if tmux.SessionExists(oldSession) {
					if err := tmux.RenameSession(oldSession, newSession); err != nil {
						return fmt.Errorf("failed to rename tmux session: %w", err)
					}
				}
				done()
			}

			baseBranch := cmd.String("base")
			if baseBranch != "" {
				done = tr.StartCtx(ctx, "record stack parent")
				if err := stack.Update(rwt.Repo.BareDir, func(s *stack.Stack) {
					s.SetParent(branch, baseBranch)
				}); err != nil {
					u.Warn(fmt.Sprintf("Failed to save stack: %v", err))
				}
				done()
			}

			meta := map[string]string{
				"from": rwt.Worktree.MatchName(),
				"path": finalPath,
			}
			if finalPath != rwt.Worktree.Path {
				meta["old_path"] = rwt.Worktree.Path
			}
			if baseBranch != "" {
				meta["base"] = baseBranch
			}
			_ = log.Append(log.Event{Action: "promote", Repo: rwt.Repo.Name, Branch: branch, Metadata: meta})

			if cdOnly {
				fmt.Println(finalPath)
				return nil
			}

			u.Success(fmt.Sprintf("Promoted %s to branch %s", u.Bold(rwt.Worktree.MatchName()), u.Bold(branch)))
			u.Info(fmt.Sprintf("  path:   %s", u.Dim(finalPath)))
			if baseBranch != "" {
				u.Info(fmt.Sprintf("  base:   %s", u.Dim(baseBranch)))
			}
			return nil
		},
	}
}

func promotionArgs(g *git.Git, first, second string) (target, branch string, explicitBranch bool) {
	if second != "" {
		return first, second, true
	}
	if first == "" {
		return "", "", false
	}
	if current, err := currentManagedWorktree(g); err == nil && current.Worktree.Detached {
		return "", first, true
	}
	return first, first, false
}

func checkGeneratedPromotionCollisions(repoName, oldDir, newDir, oldPath, newPath string) error {
	if oldDir == newDir {
		return nil
	}
	if comparablePath(oldPath) != comparablePath(newPath) && pathExists(newPath) {
		return errors.Userf("worktree path already exists: %s", newPath)
	}
	oldStatus := claude.StatusWorktreeDir(repoName, oldDir)
	newStatus := claude.StatusWorktreeDir(repoName, newDir)
	if comparablePath(oldStatus) != comparablePath(newStatus) && pathExists(newStatus) {
		return errors.Userf("status directory already exists: %s", newStatus)
	}
	oldSession := tmux.SessionNameForWorktree(repoName, oldDir)
	newSession := tmux.SessionNameForWorktree(repoName, newDir)
	if oldSession != newSession && tmux.SessionExists(newSession) {
		return errors.Userf("tmux session already exists: %s", newSession)
	}
	return nil
}

func resolvePromotionTarget(g *git.Git, repoFlag, target string) (*repoWorktree, error) {
	if target == "" {
		return currentManagedWorktree(g)
	}

	repos, err := resolveRepos(g, repoFlag)
	if err != nil {
		return nil, err
	}
	allWts := collectAllWorktrees(repos, g.Verbose)
	return findCrossRepoWorktree(allWts, target)
}

func currentManagedWorktree(g *git.Git) (*repoWorktree, error) {
	bareDir, err := requireWillowRepo(g)
	if err != nil {
		return nil, err
	}
	root, err := g.WorktreeRoot()
	if err != nil {
		return nil, errors.Userf("not inside a willow-managed worktree\n\nRun this from a detached worktree, or pass a worktree name.")
	}

	repo := repoInfo{Name: repoNameFromDir(bareDir), BareDir: bareDir}
	repoGit := &git.Git{Dir: bareDir, Verbose: g.Verbose}
	wts, err := worktree.List(repoGit)
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	cleanRoot := comparablePath(filepath.Clean(root))
	for _, wt := range filterBareWorktrees(wts) {
		if comparablePath(filepath.Clean(wt.Path)) == cleanRoot {
			return &repoWorktree{Repo: repo, Worktree: wt}, nil
		}
	}
	return nil, errors.Userf("current directory is not a willow-managed worktree")
}
