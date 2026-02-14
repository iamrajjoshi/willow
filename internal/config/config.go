package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	BaseBranch   string   `json:"baseBranch,omitempty"`
	BranchPrefix string   `json:"branchPrefix,omitempty"`
	Setup        []string `json:"setup,omitempty"`
	Teardown     []string `json:"teardown,omitempty"`
	Defaults     Defaults `json:"defaults"`
}

type Defaults struct {
	Fetch           *bool `json:"fetch,omitempty"`
	AutoSetupRemote *bool `json:"autoSetupRemote,omitempty"`
}

func BoolPtr(v bool) *bool { return &v }

func DefaultConfig() *Config {
	return &Config{
		Defaults: Defaults{
			Fetch:           BoolPtr(true),
			AutoSetupRemote: BoolPtr(true),
		},
	}
}

// WillowHome returns ~/.willow
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

func GlobalConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "willow", "config.json")
}

func LocalConfigPath(bareDir string) string {
	return filepath.Join(bareDir, "willow.json")
}

func SharedConfigPath(worktreeRoot string) string {
	return filepath.Join(worktreeRoot, ".willow", "config.json")
}

// IsWillowRepo checks if bareDir lives under ~/.willow/repos/.
// Both paths are resolved through EvalSymlinks to handle macOS /var → /private/var.
func IsWillowRepo(bareDir string) bool {
	reposDir := ReposDir()
	if resolved, err := filepath.EvalSymlinks(reposDir); err == nil {
		reposDir = resolved
	}
	if resolved, err := filepath.EvalSymlinks(bareDir); err == nil {
		bareDir = resolved
	}
	return strings.HasPrefix(bareDir, reposDir+string(filepath.Separator))
}

// ListRepos scans ~/.willow/repos/ for *.git dirs and returns repo names (without .git suffix).
func ListRepos() ([]string, error) {
	entries, err := os.ReadDir(ReposDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var repos []string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".git") {
			repos = append(repos, strings.TrimSuffix(e.Name(), ".git"))
		}
	}
	return repos, nil
}

// ResolveRepo returns the bare dir path for a named repo under ~/.willow/repos/.
func ResolveRepo(name string) (string, error) {
	dir := filepath.Join(ReposDir(), name+".git")
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("repo %q not found in %s", name, ReposDir())
	}
	return dir, nil
}

// Load resolves config by merging 3 tiers: global → shared → local.
// bareDir and worktreeRoot can be empty if unavailable (e.g. not in a repo).
func Load(bareDir, worktreeRoot string) *Config {
	cfg := DefaultConfig()

	if global, err := LoadFile(GlobalConfigPath()); err == nil {
		merge(cfg, global)
	}

	if worktreeRoot != "" {
		if shared, err := LoadFile(SharedConfigPath(worktreeRoot)); err == nil {
			merge(cfg, shared)
		}
	}

	if bareDir != "" {
		if local, err := LoadFile(LocalConfigPath(bareDir)); err == nil {
			merge(cfg, local)
		}
	}

	return cfg
}

func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// merge overlays non-zero fields from overlay onto base (mutates base).
func merge(base, overlay *Config) {
	if overlay.BaseBranch != "" {
		base.BaseBranch = overlay.BaseBranch
	}
	if overlay.BranchPrefix != "" {
		base.BranchPrefix = overlay.BranchPrefix
	}
	if overlay.Setup != nil {
		base.Setup = overlay.Setup
	}
	if overlay.Teardown != nil {
		base.Teardown = overlay.Teardown
	}
	if overlay.Defaults.Fetch != nil {
		base.Defaults.Fetch = overlay.Defaults.Fetch
	}
	if overlay.Defaults.AutoSetupRemote != nil {
		base.Defaults.AutoSetupRemote = overlay.Defaults.AutoSetupRemote
	}
}

// Save writes a config to the given path, creating directories as needed.
func Save(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
