package git

import (
	"bytes"
	"fmt"
	"io"
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

// RunStream captures stdout and returns it, while streaming stderr to the
// given writer in real-time. Useful for commands like `git fetch --progress`
// where progress output goes to stderr.
func (g *Git) RunStream(stderr io.Writer, args ...string) (string, error) {
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

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(stdout.String()), nil
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

// MergedBranches returns branches that have been merged into origin/<base>.
func (g *Git) MergedBranches(base string) ([]string, error) {
	out, err := g.Run("branch", "--merged", "origin/"+base, "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var branches []string
	for _, b := range strings.Split(out, "\n") {
		b = strings.TrimSpace(b)
		if b != "" && b != base {
			branches = append(branches, b)
		}
	}
	return branches, nil
}

// RemoteBranches returns remote branch names from origin, stripping the
// "origin/" prefix. The HEAD pointer is excluded.
func (g *Git) RemoteBranches() ([]string, error) {
	out, err := g.Run("branch", "-r", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var branches []string
	for _, b := range strings.Split(out, "\n") {
		b = strings.TrimSpace(b)
		if b == "" || b == "origin/HEAD" || strings.HasSuffix(b, "/HEAD") {
			continue
		}
		branches = append(branches, strings.TrimPrefix(b, "origin/"))
	}
	return branches, nil
}

func (g *Git) HasUnpushedCommits() (bool, error) {
	out, err := g.Run("rev-list", "--count", "@{upstream}..HEAD")
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no upstream") || strings.Contains(errMsg, "upstream") {
			return true, nil
		}
		return false, err
	}
	return strings.TrimSpace(out) != "0", nil
}
