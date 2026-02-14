package worktree

import (
	"strings"

	"github.com/iamrajjoshi/willow/internal/git"
)

type Worktree struct {
	Branch string `json:"branch"`
	Path   string `json:"path"`
	Head   string `json:"head"`
	IsBare bool   `json:"-"`
}

func List(g *git.Git) ([]Worktree, error) {
	out, err := g.Run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parsePorcelain(out), nil
}

func parsePorcelain(output string) []Worktree {
	var worktrees []Worktree
	for _, block := range strings.Split(strings.TrimSpace(output), "\n\n") {
		var wt Worktree
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				wt.Path = strings.TrimPrefix(line, "worktree ")
			case strings.HasPrefix(line, "HEAD "):
				wt.Head = strings.TrimPrefix(line, "HEAD ")
			case strings.HasPrefix(line, "branch "):
				wt.Branch = strings.TrimPrefix(line, "branch refs/heads/")
			case line == "bare":
				wt.IsBare = true
			case line == "detached":
				wt.Branch = "(detached)"
			}
		}
		if wt.Path != "" {
			worktrees = append(worktrees, wt)
		}
	}
	return worktrees
}
