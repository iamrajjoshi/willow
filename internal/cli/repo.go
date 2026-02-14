package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/iamrajjoshi/willow/internal/git"
)

// resolveBareRepo finds the bare repo directory from the current working directory.
func resolveBareRepo(g *git.Git) (string, error) {
	dir, err := g.Run("rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("not inside a git repository (run 'ww clone' first)")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return absDir, nil
}

// repoNameFromDir extracts the repo name from a bare repo path.
// e.g. ~/.willow/repos/myrepo.git → myrepo
func repoNameFromDir(bareDir string) string {
	return strings.TrimSuffix(filepath.Base(bareDir), ".git")
}

func detectDefaultBranch(g *git.Git) (string, error) {
	ref, err := g.Run("symbolic-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(ref, "refs/heads/"), nil
}
