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

	g.logCmd(args)

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

	g.logCmd(args)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (g *Git) logCmd(args []string) {
	if !g.Verbose {
		return
	}
	if g.Dir != "" {
		fmt.Fprintf(os.Stderr, "$ git -C %s %s\n", g.Dir, strings.Join(args, " "))
	} else {
		fmt.Fprintf(os.Stderr, "$ git %s\n", strings.Join(args, " "))
	}
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

// ResolveBaseBranch picks the base branch to use for operations like merge
// detection. It prefers an explicit configured value, then falls back to the
// repo's HEAD symbolic ref (typically "main" or "master"), and finally "main".
func (g *Git) ResolveBaseBranch(configured string) string {
	if configured != "" {
		return configured
	}
	if b, err := g.DefaultBranch(); err == nil && b != "" {
		return b
	}
	return "main"
}

func (g *Git) IsDirty() (bool, error) {
	out, err := g.Run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// MergedBranches returns branches that have been merged into origin/<base>.
// Branches whose tip SHA equals origin/<base> are excluded — a brand-new
// branch forked from origin/<base> has zero unique commits and would
// otherwise be reported as "merged" before any work has happened.
func (g *Git) MergedBranches(base string) ([]string, error) {
	out, err := g.Run("branch", "--merged", "origin/"+base, "--format=%(refname:short) %(objectname)")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	baseSHA, _ := g.Run("rev-parse", "origin/"+base)
	var branches []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, sha, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		if name == base {
			continue
		}
		if baseSHA != "" && sha == baseSHA {
			continue
		}
		branches = append(branches, name)
	}
	return branches, nil
}

// MergedBranchSet returns a set of branches that have been merged into origin/<base>.
func (g *Git) MergedBranchSet(base string) map[string]bool {
	merged, _ := g.MergedBranches(base)
	set := make(map[string]bool, len(merged))
	for _, b := range merged {
		set[b] = true
	}
	return set
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

// RemoteBranchExists checks if a branch exists on origin.
func (g *Git) RemoteBranchExists(branch string) bool {
	out, _ := g.Run("branch", "-r", "--list", "origin/"+branch)
	return strings.TrimSpace(out) != ""
}

// LocalBranchExists checks if a local branch exists in the repo.
func (g *Git) LocalBranchExists(branch string) bool {
	out, _ := g.Run("branch", "--list", branch)
	return strings.TrimSpace(out) != ""
}

// Rebase runs git rebase <onto> in the current directory.
func (g *Git) Rebase(onto string) error {
	_, err := g.Run("rebase", onto)
	return err
}

// RebaseAbort aborts an in-progress rebase.
func (g *Git) RebaseAbort() error {
	_, err := g.Run("rebase", "--abort")
	return err
}

// IsRebaseInProgress checks if a rebase is currently in progress.
func (g *Git) IsRebaseInProgress() bool {
	gitDir, err := g.Run("rev-parse", "--git-dir")
	if err != nil {
		return false
	}
	for _, dir := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(gitDir, dir)); err == nil {
			return true
		}
	}
	return false
}

// CommitsAhead returns the number of commits HEAD is ahead of base.
func (g *Git) CommitsAhead(base string) (int, error) {
	out, err := g.Run("rev-list", "--count", base+"..HEAD")
	if err != nil {
		return 0, err
	}
	var count int
	if _, err := fmt.Sscanf(strings.TrimSpace(out), "%d", &count); err != nil {
		return 0, fmt.Errorf("failed to parse commit count %q: %w", out, err)
	}
	return count, nil
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

// ParseShortstat converts git diff --shortstat output into a compact summary
// like "3f +42 -12".
func ParseShortstat(out string) string {
	if out == "" {
		return "--"
	}
	// "3 files changed, 42 insertions(+), 12 deletions(-)"
	parts := strings.Split(out, ",")
	files := ""
	ins := ""
	del := ""
	for _, p := range parts {
		p = strings.TrimSpace(p)
		fields := strings.Fields(p)
		if len(fields) >= 2 {
			switch {
			case strings.Contains(p, "file"):
				files = fields[0] + "f"
			case strings.Contains(p, "insertion"):
				ins = "+" + fields[0]
			case strings.Contains(p, "deletion"):
				del = "-" + fields[0]
			}
		}
	}
	result := files
	if ins != "" {
		result += " " + ins
	}
	if del != "" {
		result += " " + del
	}
	if result == "" {
		return "--"
	}
	return result
}
