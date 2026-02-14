package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	BaseBranch   string   `json:"baseBranch,omitempty"`
	BranchPrefix string   `json:"branchPrefix,omitempty"`
	Setup        []string `json:"setup,omitempty"`
	Teardown     []string `json:"teardown,omitempty"`
	Defaults     Defaults `json:"defaults"`
}

type Defaults struct {
	Fetch           bool `json:"fetch"`
	AutoSetupRemote bool `json:"autoSetupRemote"`
}

// ~/.willow
func WillowHome() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".willow")
}

func ReposDir() string {
	return filepath.Join(WillowHome(), "repos")
}

func WorktreesDir() string {
	return filepath.Join(WillowHome(), "worktrees")
}
