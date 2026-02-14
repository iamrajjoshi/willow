package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Git struct {
	Dir     string
	Verbose bool
}

func (g *Git) Run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if g.Dir != "" {
		cmd.Dir = g.Dir
	}

	if g.Verbose {
		if g.Dir != "" {
			fmt.Fprintf(os.Stderr, "$ git -C %s %s\n", g.Dir, strings.Join(args, " "))
		} else {
			fmt.Fprintf(os.Stderr, "$ git %s\n", strings.Join(args, " "))
		}
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *Git) BareRepoDir() (string, error) {
	dir, err := g.Run("rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("not inside a git repository")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return absDir, nil
}

func (g *Git) WorktreeRoot() (string, error) {
	dir, err := g.Run("rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not inside a git worktree")
	}
	return dir, nil
}

func (g *Git) DefaultBranch() (string, error) {
	ref, err := g.Run("symbolic-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(ref, "refs/heads/"), nil
}

func (g *Git) IsDirty() (bool, error) {
	out, err := g.Run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

func (g *Git) HasUnpushedCommits() (bool, error) {
	out, err := g.Run("rev-list", "--count", "@{upstream}..HEAD")
	if err != nil {
		// No upstream set — treat as unpushed
		return true, nil
	}
	return strings.TrimSpace(out) != "0", nil
}
