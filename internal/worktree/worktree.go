package worktree

import "github.com/iamrajjoshi/willow/internal/config"

type Worktree struct {
	Branch string
	Path   string
	Repo   string
}

func List(repoName string) ([]Worktree, error) {
	_ = config.WorktreesDir()
	// TODO: implement via git worktree list --porcelain
	return nil, nil
}
