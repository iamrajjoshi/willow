package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.BaseBranch != "" {
		t.Errorf("BaseBranch = %q, want empty", cfg.BaseBranch)
	}
	if cfg.BranchPrefix != "" {
		t.Errorf("BranchPrefix = %q, want empty", cfg.BranchPrefix)
	}
	if cfg.Defaults.Fetch == nil || !*cfg.Defaults.Fetch {
		t.Error("Defaults.Fetch should be true")
	}
	if cfg.Defaults.AutoSetupRemote == nil || !*cfg.Defaults.AutoSetupRemote {
		t.Error("Defaults.AutoSetupRemote should be true")
	}
}

func TestMerge_StringOverride(t *testing.T) {
	base := &Config{BaseBranch: "main", BranchPrefix: "alice"}
	overlay := &Config{BaseBranch: "develop"}

	merge(base, overlay)

	if base.BaseBranch != "develop" {
		t.Errorf("BaseBranch = %q, want %q", base.BaseBranch, "develop")
	}
	if base.BranchPrefix != "alice" {
		t.Errorf("BranchPrefix = %q, want %q (should be unchanged)", base.BranchPrefix, "alice")
	}
}

func TestMerge_EmptyStringDoesNotOverride(t *testing.T) {
	base := &Config{BaseBranch: "main"}
	overlay := &Config{BaseBranch: ""}

	merge(base, overlay)

	if base.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want %q", base.BaseBranch, "main")
	}
}

func TestMerge_SliceOverride(t *testing.T) {
	base := &Config{Setup: []string{"npm install"}}
	overlay := &Config{Setup: []string{"yarn install", "yarn build"}}

	merge(base, overlay)

	if len(base.Setup) != 2 || base.Setup[0] != "yarn install" {
		t.Errorf("Setup = %v, want [yarn install, yarn build]", base.Setup)
	}
}

func TestMerge_EmptySliceOverridesNil(t *testing.T) {
	base := &Config{Setup: []string{"npm install"}}
	// Explicit empty slice means "clear the setup commands"
	overlay := &Config{Setup: []string{}}

	merge(base, overlay)

	if len(base.Setup) != 0 {
		t.Errorf("Setup = %v, want empty (explicit empty slice should clear)", base.Setup)
	}
}

func TestMerge_NilSliceDoesNotOverride(t *testing.T) {
	base := &Config{Setup: []string{"npm install"}}
	overlay := &Config{} // Setup is nil

	merge(base, overlay)

	if len(base.Setup) != 1 || base.Setup[0] != "npm install" {
		t.Errorf("Setup = %v, want [npm install]", base.Setup)
	}
}

func TestMerge_BoolPointerOverride(t *testing.T) {
	base := DefaultConfig() // fetch=true, autoSetupRemote=true
	overlay := &Config{
		Defaults: Defaults{Fetch: BoolPtr(false)},
	}

	merge(base, overlay)

	if *base.Defaults.Fetch != false {
		t.Error("Defaults.Fetch should be false after override")
	}
	if *base.Defaults.AutoSetupRemote != true {
		t.Error("Defaults.AutoSetupRemote should remain true (not overridden)")
	}
}

func TestMerge_NilBoolDoesNotOverride(t *testing.T) {
	base := DefaultConfig()
	overlay := &Config{} // Defaults.Fetch is nil

	merge(base, overlay)

	if *base.Defaults.Fetch != true {
		t.Error("Defaults.Fetch should remain true when overlay is nil")
	}
}

func TestLoadFile_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"baseBranch": "develop", "defaults": {"fetch": false}}`), 0o644)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}
	if cfg.BaseBranch != "develop" {
		t.Errorf("BaseBranch = %q, want %q", cfg.BaseBranch, "develop")
	}
	if cfg.Defaults.Fetch == nil || *cfg.Defaults.Fetch != false {
		t.Error("Defaults.Fetch should be false")
	}
}

func TestLoadFile_MissingFile(t *testing.T) {
	_, err := LoadFile("/nonexistent/config.json")
	if err == nil {
		t.Error("LoadFile() should return error for missing file")
	}
}

func TestLoadFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{not json}`), 0o644)

	_, err := LoadFile(path)
	if err == nil {
		t.Error("LoadFile() should return error for invalid JSON")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")

	cfg := &Config{
		BaseBranch:   "main",
		BranchPrefix: "raj",
		Setup:        []string{"npm install"},
		Defaults: Defaults{
			Fetch:           BoolPtr(false),
			AutoSetupRemote: BoolPtr(true),
		},
	}

	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}

	if loaded.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want %q", loaded.BaseBranch, "main")
	}
	if loaded.BranchPrefix != "raj" {
		t.Errorf("BranchPrefix = %q, want %q", loaded.BranchPrefix, "raj")
	}
	if len(loaded.Setup) != 1 || loaded.Setup[0] != "npm install" {
		t.Errorf("Setup = %v, want [npm install]", loaded.Setup)
	}
	if *loaded.Defaults.Fetch != false {
		t.Error("Defaults.Fetch should be false")
	}
	if *loaded.Defaults.AutoSetupRemote != true {
		t.Error("Defaults.AutoSetupRemote should be true")
	}
}

func TestLoad_TwoTierMerge(t *testing.T) {
	tmp := t.TempDir()

	// We can't override GlobalConfigPath() easily since it uses os.UserHomeDir.
	// Instead, test the merge behavior by calling LoadFile + merge directly,
	// which is what Load does internally.

	// Simulate global: baseBranch=main, fetch=true, setup=[npm install]
	globalDir := filepath.Join(tmp, "global")
	os.MkdirAll(globalDir, 0o755)
	os.WriteFile(filepath.Join(globalDir, "config.json"), []byte(`{
		"baseBranch": "main",
		"setup": ["npm install"],
		"defaults": {"fetch": true}
	}`), 0o644)

	// Simulate local: baseBranch=develop, fetch=false (overrides global)
	localDir := filepath.Join(tmp, "local")
	os.MkdirAll(localDir, 0o755)
	os.WriteFile(filepath.Join(localDir, "config.json"), []byte(`{
		"baseBranch": "develop",
		"defaults": {"fetch": false}
	}`), 0o644)

	cfg := DefaultConfig()

	global, _ := LoadFile(filepath.Join(globalDir, "config.json"))
	merge(cfg, global)

	local, _ := LoadFile(filepath.Join(localDir, "config.json"))
	merge(cfg, local)

	if cfg.BaseBranch != "develop" {
		t.Errorf("BaseBranch = %q, want %q", cfg.BaseBranch, "develop")
	}
	if len(cfg.Setup) != 1 || cfg.Setup[0] != "npm install" {
		t.Errorf("Setup = %v, want [npm install]", cfg.Setup)
	}
	if *cfg.Defaults.Fetch != false {
		t.Error("Defaults.Fetch should be false (local override)")
	}
	if *cfg.Defaults.AutoSetupRemote != true {
		t.Error("Defaults.AutoSetupRemote should remain true (no overrides)")
	}
}

func TestLoad_NoConfigFiles(t *testing.T) {
	cfg := Load("/nonexistent/bare")

	if cfg.BaseBranch != "" {
		t.Errorf("BaseBranch = %q, want empty", cfg.BaseBranch)
	}
	if *cfg.Defaults.Fetch != true {
		t.Error("Defaults.Fetch should be true (default)")
	}
	if *cfg.Defaults.AutoSetupRemote != true {
		t.Error("Defaults.AutoSetupRemote should be true (default)")
	}
}

func TestLoad_EmptyBareDir(t *testing.T) {
	cfg := Load("")

	if *cfg.Defaults.Fetch != true {
		t.Error("Defaults.Fetch should be true")
	}
}

func TestWillowHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := WillowHome()
	want := filepath.Join(home, ".willow")
	if got != want {
		t.Errorf("WillowHome() = %q, want %q", got, want)
	}
}

func TestReposDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := ReposDir()
	want := filepath.Join(home, ".willow", "repos")
	if got != want {
		t.Errorf("ReposDir() = %q, want %q", got, want)
	}
}

func TestWorktreesDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := WorktreesDir()
	want := filepath.Join(home, ".willow", "worktrees")
	if got != want {
		t.Errorf("WorktreesDir() = %q, want %q", got, want)
	}
}

func TestTrashDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := TrashDir()
	want := filepath.Join(home, ".willow", "trash")
	if got != want {
		t.Errorf("TrashDir() = %q, want %q", got, want)
	}
}

func TestListRepos_Empty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repos, err := ListRepos()
	if err != nil {
		t.Fatalf("ListRepos() error: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func TestListRepos_WithRepos(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	reposDir := filepath.Join(home, ".willow", "repos")
	os.MkdirAll(filepath.Join(reposDir, "alpha.git"), 0o755)
	os.MkdirAll(filepath.Join(reposDir, "beta.git"), 0o755)
	// Non-.git dir should be ignored
	os.MkdirAll(filepath.Join(reposDir, "notarepo"), 0o755)

	repos, err := ListRepos()
	if err != nil {
		t.Fatalf("ListRepos() error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	names := map[string]bool{}
	for _, r := range repos {
		names[r] = true
	}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("expected alpha and beta, got %v", names)
	}
}

func TestResolveRepo_Found(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoDir := filepath.Join(home, ".willow", "repos", "myrepo.git")
	os.MkdirAll(repoDir, 0o755)

	got, err := ResolveRepo("myrepo")
	if err != nil {
		t.Fatalf("ResolveRepo error: %v", err)
	}
	if got != repoDir {
		t.Errorf("ResolveRepo = %q, want %q", got, repoDir)
	}
}

func TestResolveRepo_NotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, err := ResolveRepo("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent repo")
	}
}

func TestIsWillowRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	reposDir := filepath.Join(home, ".willow", "repos")
	repoDir := filepath.Join(reposDir, "myrepo.git")
	os.MkdirAll(repoDir, 0o755)

	if !IsWillowRepo(repoDir) {
		t.Error("path under repos dir should be a willow repo")
	}
	if IsWillowRepo("/some/other/path") {
		t.Error("arbitrary path should not be a willow repo")
	}
}

func TestResolveRepoFromDir_InsideWorktree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoDir := filepath.Join(home, ".willow", "repos", "myrepo.git")
	os.MkdirAll(repoDir, 0o755)

	wtDir := filepath.Join(home, ".willow", "worktrees", "myrepo", "feature-auth")
	os.MkdirAll(wtDir, 0o755)

	got, ok := ResolveRepoFromDir(wtDir)
	if !ok {
		t.Fatal("expected ok=true for path inside worktrees")
	}
	if got != repoDir {
		t.Errorf("ResolveRepoFromDir = %q, want %q", got, repoDir)
	}
}

func TestResolveRepoFromDir_OutsideWorktrees(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".willow", "worktrees"), 0o755)

	_, ok := ResolveRepoFromDir(home)
	if ok {
		t.Error("expected ok=false for path outside worktrees")
	}
}

func TestResolveRepoFromDir_WorktreesRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".willow", "worktrees"), 0o755)

	_, ok := ResolveRepoFromDir(filepath.Join(home, ".willow", "worktrees"))
	if ok {
		t.Error("worktrees root itself should return false")
	}
}

func TestMerge_TmuxConfig(t *testing.T) {
	base := DefaultConfig()
	overlay := &Config{
		Tmux: TmuxConfig{
			ReloadInterval: 5,
			Notification:   BoolPtr(true),
			NotifyCommand:  "say done",
			Layout: []string{
				"split-window -h",
				"select-layout even-horizontal",
			},
		},
	}

	merge(base, overlay)

	if base.Tmux.ReloadInterval != 5 {
		t.Errorf("ReloadInterval = %d, want 5", base.Tmux.ReloadInterval)
	}
	if base.Tmux.Notification == nil || !*base.Tmux.Notification {
		t.Error("Notification should be true")
	}
	if base.Tmux.NotifyCommand != "say done" {
		t.Errorf("NotifyCommand = %q, want %q", base.Tmux.NotifyCommand, "say done")
	}
	if len(base.Tmux.Layout) != 2 || base.Tmux.Layout[0] != "split-window -h" {
		t.Errorf("Layout = %v, want [split-window -h, select-layout even-horizontal]", base.Tmux.Layout)
	}
}

func TestMerge_TmuxConfigPartial(t *testing.T) {
	base := &Config{
		Tmux: TmuxConfig{
			ReloadInterval: 3,
			NotifyCommand:  "original",
		},
	}
	overlay := &Config{
		Tmux: TmuxConfig{
			NotifyCommand: "updated",
		},
	}

	merge(base, overlay)

	if base.Tmux.ReloadInterval != 3 {
		t.Errorf("ReloadInterval = %d, want 3 (unchanged)", base.Tmux.ReloadInterval)
	}
	if base.Tmux.NotifyCommand != "updated" {
		t.Errorf("NotifyCommand = %q, want %q", base.Tmux.NotifyCommand, "updated")
	}
}

func TestMerge_PostCheckoutHook(t *testing.T) {
	base := &Config{}
	overlay := &Config{PostCheckoutHook: ".hooks/post-checkout"}

	merge(base, overlay)

	if base.PostCheckoutHook != ".hooks/post-checkout" {
		t.Errorf("PostCheckoutHook = %q, want %q", base.PostCheckoutHook, ".hooks/post-checkout")
	}
}

func TestMerge_TeardownHooks(t *testing.T) {
	base := &Config{Teardown: []string{"cleanup.sh"}}
	overlay := &Config{Teardown: []string{"new-cleanup.sh"}}

	merge(base, overlay)

	if len(base.Teardown) != 1 || base.Teardown[0] != "new-cleanup.sh" {
		t.Errorf("Teardown = %v, want [new-cleanup.sh]", base.Teardown)
	}
}

func TestMerge_SwitcherPreview_NilDoesNotOverride(t *testing.T) {
	base := &Config{Tmux: TmuxConfig{SwitcherPreview: BoolPtr(false)}}
	overlay := &Config{}

	merge(base, overlay)

	if base.Tmux.SwitcherPreview == nil || *base.Tmux.SwitcherPreview != false {
		t.Error("SwitcherPreview should remain false when overlay is nil")
	}
}

func TestMerge_SwitcherPreview_Override(t *testing.T) {
	base := &Config{Tmux: TmuxConfig{SwitcherPreview: BoolPtr(true)}}
	overlay := &Config{Tmux: TmuxConfig{SwitcherPreview: BoolPtr(false)}}

	merge(base, overlay)

	if base.Tmux.SwitcherPreview == nil || *base.Tmux.SwitcherPreview != false {
		t.Error("SwitcherPreview should be false after override")
	}
}

func TestMerge_NotifyWaitCommand(t *testing.T) {
	base := &Config{Tmux: TmuxConfig{NotifyWaitCommand: "original"}}
	overlay := &Config{Tmux: TmuxConfig{NotifyWaitCommand: "updated"}}

	merge(base, overlay)

	if base.Tmux.NotifyWaitCommand != "updated" {
		t.Errorf("NotifyWaitCommand = %q, want %q", base.Tmux.NotifyWaitCommand, "updated")
	}
}

func TestMerge_Panes(t *testing.T) {
	base := &Config{Tmux: TmuxConfig{Panes: []PaneConfig{{Command: "old"}}}}
	overlay := &Config{Tmux: TmuxConfig{Panes: []PaneConfig{{}, {Command: "dev sync --only install"}}}}

	merge(base, overlay)

	if len(base.Tmux.Panes) != 2 || base.Tmux.Panes[1].Command != "dev sync --only install" {
		t.Errorf("Panes = %v, want [{} {dev sync --only install}]", base.Tmux.Panes)
	}
}

func TestMerge_Panes_NilDoesNotOverride(t *testing.T) {
	base := &Config{Tmux: TmuxConfig{Panes: []PaneConfig{{Command: "cd website"}}}}
	overlay := &Config{}

	merge(base, overlay)

	if len(base.Tmux.Panes) != 1 || base.Tmux.Panes[0].Command != "cd website" {
		t.Errorf("Panes = %v, want [{cd website}]", base.Tmux.Panes)
	}
}

func TestPanes_JSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		Tmux: TmuxConfig{
			Layout: []string{"split-window -h"},
			Panes:  []PaneConfig{{}, {Command: "dev sync"}},
		},
	}

	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}

	if len(loaded.Tmux.Panes) != 2 {
		t.Fatalf("Panes length = %d, want 2", len(loaded.Tmux.Panes))
	}
	if loaded.Tmux.Panes[0].Command != "" {
		t.Errorf("Panes[0].Command = %q, want empty", loaded.Tmux.Panes[0].Command)
	}
	if loaded.Tmux.Panes[1].Command != "dev sync" {
		t.Errorf("Panes[1].Command = %q, want %q", loaded.Tmux.Panes[1].Command, "dev sync")
	}
}

func TestMerge_NotifyWaitCommand_EmptyDoesNotOverride(t *testing.T) {
	base := &Config{Tmux: TmuxConfig{NotifyWaitCommand: "original"}}
	overlay := &Config{}

	merge(base, overlay)

	if base.Tmux.NotifyWaitCommand != "original" {
		t.Errorf("NotifyWaitCommand = %q, want %q", base.Tmux.NotifyWaitCommand, "original")
	}
}
