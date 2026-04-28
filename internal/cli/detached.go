package cli

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/tmux"
	"github.com/iamrajjoshi/willow/internal/worktree"
)

var generatedDetachedDirPattern = regexp.MustCompile(`^detached-[0-9a-f]{7}(-[0-9]+)?$`)

func isGeneratedDetachedDirName(name string) bool {
	return generatedDetachedDirPattern.MatchString(name)
}

func resolveDetachedCommit(repoGit *git.Git, ref string) (string, error) {
	head, err := repoGit.Run("rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("failed to resolve detached commit for %s: %w", ref, err)
	}
	return head, nil
}

func generatedDetachedWorktreeDirName(repoName, head string) string {
	base := "detached-" + worktree.ShortHead(head)
	for i := 1; ; i++ {
		candidate := base
		if i > 1 {
			candidate = fmt.Sprintf("%s-%d", base, i)
		}
		if detachedGeneratedNameAvailable(repoName, candidate) {
			return candidate
		}
	}
}

func detachedGeneratedNameAvailable(repoName, dirName string) bool {
	wtPath := filepath.Join(config.WorktreesDir(), repoName, dirName)
	if pathExists(wtPath) {
		return false
	}
	if pathExists(claude.StatusWorktreeDir(repoName, dirName)) {
		return false
	}
	return !tmux.SessionExists(tmux.SessionNameForWorktree(repoName, dirName))
}
