package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errors"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/log"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/tmux"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/urfave/cli/v3"
)

func renameCmd() *cli.Command {
	return &cli.Command{
		Name:          "rename",
		Aliases:       []string{"mv"},
		Usage:         "Rename a worktree",
		ShellComplete: completeWorktreesWithFlag,
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "target",
				UsageText: "[worktree]",
			},
			&cli.StringArg{
				Name:      "name",
				UsageText: "<new-name>",
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target a willow-managed repo by name",
			},
			&cli.BoolFlag{
				Name:  "remote",
				Usage: "Push the new branch and delete the old remote branch",
			},
			&cli.BoolFlag{
				Name:  "cd",
				Usage: "Print the new current directory when the current worktree moved",
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

			target, newName, err := renameArgs(g, cmd.StringArg("target"), cmd.StringArg("name"))
			if err != nil {
				return err
			}

			done := tr.StartCtx(ctx, "resolve rename target")
			rwt, err := resolveRenameTarget(g, cmd.String("repo"), target)
			if err != nil {
				return err
			}
			done()

			done = tr.StartCtx(ctx, "load config")
			cfg := config.Load(rwt.Repo.BareDir)
			done()

			repoGit := &git.Git{Dir: rwt.Repo.BareDir, Verbose: g.Verbose}
			plan, err := buildRenamePlan(repoGit, cfg, *rwt, newName, cmd.Bool("remote"))
			if err != nil {
				return err
			}

			if plan.RemoteRename {
				if err := prepareRemoteRename(repoGit, plan); err != nil {
					return err
				}
				if plan.OldRemoteExist {
					u.Warn(fmt.Sprintf("Remote rename requested: will push %s/%s and delete %s/%s", plan.Remote, plan.NewBranch, plan.Remote, plan.OldBranch))
				} else {
					u.Warn(fmt.Sprintf("Remote rename requested: will push %s/%s", plan.Remote, plan.NewBranch))
				}
			}

			cdPath := renameCDPath(plan.OldPath, plan.NewPath)

			if err := executeRenamePlan(ctx, tr, u, repoGit, plan, g.Verbose); err != nil {
				return err
			}

			if cdOnly {
				if cdPath != "" {
					fmt.Println(cdPath)
				}
				return nil
			}

			u.Success(fmt.Sprintf("Renamed %s to %s", u.Bold(plan.OldLabel), u.Bold(plan.NewLabel)))
			u.Info(fmt.Sprintf("  path:   %s", u.Dim(plan.NewPath)))
			return nil
		},
	}
}

type renamePlan struct {
	RepoName       string
	OldLabel       string
	NewLabel       string
	OldBranch      string
	NewBranch      string
	OldDir         string
	NewDir         string
	OldPath        string
	NewPath        string
	Detached       bool
	Remote         string
	OldRemoteExist bool
	RemoteRename   bool
}

func renameArgs(g *git.Git, first, second string) (target, newName string, err error) {
	if second != "" {
		return first, second, nil
	}
	if first == "" {
		return "", "", errors.Userf("new name is required\n\nUsage: ww rename [worktree] <new-name>")
	}
	if _, err := currentManagedWorktree(g); err != nil {
		return "", "", errors.Userf("worktree and new name are required outside a willow worktree\n\nUsage: ww rename <worktree> <new-name>")
	}
	return "", first, nil
}

func resolveRenameTarget(g *git.Git, repoFlag, target string) (*repoWorktree, error) {
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

func buildRenamePlan(repoGit *git.Git, cfg *config.Config, rwt repoWorktree, newName string, remoteRename bool) (*renamePlan, error) {
	if strings.TrimSpace(newName) == "" {
		return nil, errors.Userf("new name is required\n\nUsage: ww rename [worktree] <new-name>")
	}

	wt := rwt.Worktree
	oldDir := filepath.Base(wt.Path)
	newBranch := newName
	var newDir string

	if wt.Detached {
		if remoteRename {
			return nil, errors.Userf("--remote cannot be used with a detached worktree")
		}
		var err error
		newDir, err = detachedWorktreeDirName(newName)
		if err != nil {
			return nil, err
		}
	} else {
		if err := rejectProtectedBranchRename(repoGit, cfg, wt.Branch); err != nil {
			return nil, err
		}
		if cfg.BranchPrefix != "" && !strings.HasPrefix(newBranch, cfg.BranchPrefix+"/") {
			newBranch = cfg.BranchPrefix + "/" + newBranch
		}
		newDir = worktreeDirName(newBranch)
		if err := validateRenameDirName(newDir); err != nil {
			return nil, err
		}
	}

	newPath := filepath.Join(filepath.Dir(wt.Path), newDir)
	plan := &renamePlan{
		RepoName:     rwt.Repo.Name,
		OldLabel:     wt.MatchName(),
		NewLabel:     newName,
		OldBranch:    wt.Branch,
		NewBranch:    newBranch,
		OldDir:       oldDir,
		NewDir:       newDir,
		OldPath:      wt.Path,
		NewPath:      newPath,
		Detached:     wt.Detached,
		RemoteRename: remoteRename,
	}
	if !wt.Detached {
		plan.NewLabel = newBranch
		plan.Remote = branchRemote(repoGit, wt.Branch)
		plan.OldRemoteExist = remoteBranchExists(repoGit, plan.Remote, wt.Branch)
	}

	if plan.OldBranch == plan.NewBranch && comparablePath(plan.OldPath) == comparablePath(plan.NewPath) {
		return nil, errors.Userf("worktree is already named %q", newName)
	}

	if err := checkRenameCollisions(repoGit, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func rejectProtectedBranchRename(repoGit *git.Git, cfg *config.Config, branch string) error {
	protected := map[string]bool{}
	if cfg.BaseBranch != "" {
		protected[cfg.BaseBranch] = true
	}
	if defaultBranch, err := repoGit.DefaultBranch(); err == nil && defaultBranch != "" {
		protected[defaultBranch] = true
	}
	protected[repoGit.ResolveBaseBranch(cfg.BaseBranch)] = true
	if protected[branch] {
		return errors.Userf("refusing to rename protected base branch %q", branch)
	}
	return nil
}

func validateRenameDirName(dirName string) error {
	if dirName == "" || dirName == "." || dirName == ".." {
		return errors.Userf("invalid worktree name %q", dirName)
	}
	return nil
}

func checkRenameCollisions(repoGit *git.Git, plan *renamePlan) error {
	if !plan.Detached && plan.OldBranch != plan.NewBranch && repoGit.LocalBranchExists(plan.NewBranch) {
		return errors.Userf("branch %q already exists", plan.NewBranch)
	}
	if comparablePath(plan.OldPath) != comparablePath(plan.NewPath) && pathExists(plan.NewPath) {
		return errors.Userf("worktree path already exists: %s", plan.NewPath)
	}

	oldStatus := claude.StatusWorktreeDir(plan.RepoName, plan.OldDir)
	newStatus := claude.StatusWorktreeDir(plan.RepoName, plan.NewDir)
	if comparablePath(oldStatus) != comparablePath(newStatus) && pathExists(newStatus) {
		return errors.Userf("status directory already exists: %s", newStatus)
	}

	oldSession := tmux.SessionNameForWorktree(plan.RepoName, plan.OldDir)
	newSession := tmux.SessionNameForWorktree(plan.RepoName, plan.NewDir)
	if oldSession != newSession && tmux.SessionExists(newSession) {
		return errors.Userf("tmux session already exists: %s", newSession)
	}
	return nil
}

func executeRenamePlan(ctx context.Context, tr *trace.Tracer, u interface{ Warn(string) }, repoGit *git.Git, plan *renamePlan, verbose bool) error {
	if !plan.Detached && plan.OldBranch != plan.NewBranch {
		done := tr.StartCtx(ctx, "git branch -m")
		if _, err := repoGit.Run("branch", "-m", plan.OldBranch, plan.NewBranch); err != nil {
			return fmt.Errorf("failed to rename branch: %w", err)
		}
		done()

		if err := retargetRenamedUpstream(repoGit, plan.OldBranch, plan.NewBranch); err != nil {
			return err
		}
	}

	if comparablePath(plan.OldPath) != comparablePath(plan.NewPath) {
		done := tr.StartCtx(ctx, "git worktree move")
		if _, err := repoGit.Run("worktree", "move", plan.OldPath, plan.NewPath); err != nil {
			return fmt.Errorf("failed to move worktree: %w", err)
		}
		done()
	}

	if plan.OldDir != plan.NewDir {
		done := tr.StartCtx(ctx, "move status dir")
		if err := claude.MoveStatusDir(plan.RepoName, plan.OldDir, plan.NewDir); err != nil {
			return fmt.Errorf("failed to move status directory: %w", err)
		}
		done()

		done = tr.StartCtx(ctx, "rename tmux session")
		oldSession := tmux.SessionNameForWorktree(plan.RepoName, plan.OldDir)
		newSession := tmux.SessionNameForWorktree(plan.RepoName, plan.NewDir)
		if tmux.SessionExists(oldSession) {
			if err := tmux.RenameSession(oldSession, newSession); err != nil {
				return fmt.Errorf("failed to rename tmux session: %w", err)
			}
		}
		done()
	}

	if !plan.Detached && plan.OldBranch != plan.NewBranch {
		done := tr.StartCtx(ctx, "rename stack branch")
		if err := stack.Update(repoGit.Dir, func(s *stack.Stack) {
			s.Rename(plan.OldBranch, plan.NewBranch)
		}); err != nil {
			u.Warn(fmt.Sprintf("Failed to save stack: %v", err))
		}
		done()
	}

	if !plan.Detached && plan.RemoteRename {
		done := tr.StartCtx(ctx, "git push renamed branch")
		wtGit := &git.Git{Dir: plan.NewPath, Verbose: verbose}
		if _, err := wtGit.Run("push", "-u", plan.Remote, plan.NewBranch); err != nil {
			return fmt.Errorf("failed to push renamed branch: %w", err)
		}
		done()

		if plan.OldRemoteExist {
			done = tr.StartCtx(ctx, "git delete old remote branch")
			if _, err := repoGit.Run("push", plan.Remote, "--delete", plan.OldBranch); err != nil {
				return fmt.Errorf("failed to delete old remote branch: %w", err)
			}
			done()
		}
	} else if !plan.Detached && plan.OldRemoteExist {
		u.Warn(fmt.Sprintf("Remote branch %s/%s was left unchanged. Use --remote to push %s/%s and delete %s/%s.", plan.Remote, plan.OldBranch, plan.Remote, plan.NewBranch, plan.Remote, plan.OldBranch))
	}

	meta := map[string]string{
		"from":        plan.OldLabel,
		"old_path":    plan.OldPath,
		"path":        plan.NewPath,
		"remote":      strconv.FormatBool(plan.RemoteRename),
		"remote_name": plan.Remote,
	}
	_ = log.Append(log.Event{Action: "rename", Repo: plan.RepoName, Branch: plan.NewLabel, Metadata: meta})
	return nil
}

func retargetRenamedUpstream(repoGit *git.Git, oldBranch, newBranch string) error {
	key := "branch." + newBranch + ".merge"
	if gitConfigGet(repoGit, key) != "refs/heads/"+oldBranch {
		return nil
	}
	if _, err := repoGit.Run("config", key, "refs/heads/"+newBranch); err != nil {
		return fmt.Errorf("failed to update upstream tracking for %q: %w", newBranch, err)
	}
	return nil
}

func branchRemote(repoGit *git.Git, branch string) string {
	remote := gitConfigGet(repoGit, "branch."+branch+".remote")
	if remote == "" || remote == "." {
		return "origin"
	}
	return remote
}

func gitConfigGet(g *git.Git, key string) string {
	out, err := g.Run("config", "--get", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func remoteBranchExists(repoGit *git.Git, remote, branch string) bool {
	out, _ := repoGit.Run("branch", "-r", "--list", remote+"/"+branch)
	return strings.TrimSpace(out) != ""
}

func prepareRemoteRename(repoGit *git.Git, plan *renamePlan) error {
	exists, err := remoteBranchExistsLive(repoGit, plan.Remote, plan.NewBranch)
	if err != nil {
		return err
	}
	if exists {
		return errors.Userf("remote branch %s/%s already exists", plan.Remote, plan.NewBranch)
	}

	oldExists, err := fetchRemoteBranchRef(repoGit, plan.Remote, plan.OldBranch)
	if err != nil {
		return err
	}
	plan.OldRemoteExist = oldExists
	if !oldExists {
		return nil
	}
	return ensureRemoteBranchSafeToDelete(repoGit, plan.Remote, plan.OldBranch)
}

func remoteBranchExistsLive(repoGit *git.Git, remote, branch string) (bool, error) {
	ref := "refs/heads/" + branch
	out, err := repoGit.Run("ls-remote", "--heads", remote, ref)
	if err != nil {
		return false, fmt.Errorf("failed to check remote branch %s/%s: %w", remote, branch, err)
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == ref {
			return true, nil
		}
	}
	return false, nil
}

func fetchRemoteBranchRef(repoGit *git.Git, remote, branch string) (bool, error) {
	exists, err := remoteBranchExistsLive(repoGit, remote, branch)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	refspec := "+refs/heads/" + branch + ":refs/remotes/" + remote + "/" + branch
	if _, err := repoGit.Run("fetch", remote, refspec); err != nil {
		return false, fmt.Errorf("failed to fetch %s/%s before remote rename: %w", remote, branch, err)
	}
	return true, nil
}

func ensureRemoteBranchSafeToDelete(repoGit *git.Git, remote, branch string) error {
	out, err := repoGit.Run("rev-list", "--count", "refs/heads/"+branch+"..refs/remotes/"+remote+"/"+branch)
	if err != nil {
		return fmt.Errorf("failed to compare %s/%s before deleting it: %w", remote, branch, err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return fmt.Errorf("failed to parse remote-only commit count %q: %w", out, err)
	}
	if count > 0 {
		return errors.Userf("remote branch %s/%s has commits missing from local %s\n\nFetch and reconcile it before using --remote.", remote, branch, branch)
	}
	return nil
}

func renameCDPath(oldPath, newPath string) string {
	cwd, err := os.Getwd()
	if err != nil || !pathWithin(oldPath, cwd) {
		return ""
	}
	rel, err := filepath.Rel(comparablePath(oldPath), comparablePath(cwd))
	if err != nil || rel == "." {
		return newPath
	}
	return filepath.Join(newPath, rel)
}
