package worktree

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/iamrajjoshi/willow/internal/git"
)

const DetachedBranch = "(detached)"

type Worktree struct {
	Branch   string `json:"branch"`
	Path     string `json:"path"`
	Head     string `json:"head"`
	Detached bool   `json:"detached,omitempty"`
	IsBare   bool   `json:"-"`
}

func (wt Worktree) DirName() string {
	return filepath.Base(wt.Path)
}

func (wt Worktree) DisplayName() string {
	if !wt.Detached {
		return wt.Branch
	}
	if wt.Head == "" {
		return fmt.Sprintf("%s [detached]", wt.DirName())
	}
	return fmt.Sprintf("%s [detached %s]", wt.DirName(), ShortHead(wt.Head))
}

func (wt Worktree) MatchName() string {
	if wt.Detached {
		return wt.DirName()
	}
	return wt.Branch
}

func ShortHead(head string) string {
	if len(head) <= 7 {
		return head
	}
	return head[:7]
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
				wt.Branch = DetachedBranch
				wt.Detached = true
			}
		}
		if wt.Path != "" {
			worktrees = append(worktrees, wt)
		}
	}
	return worktrees
}
