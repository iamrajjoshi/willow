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
		Defaults: Defaults{Fetch: boolPtr(false)},
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

	cfg, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile() error: %v", err)
	}
	if cfg.BaseBranch != "develop" {
		t.Errorf("BaseBranch = %q, want %q", cfg.BaseBranch, "develop")
	}
	if cfg.Defaults.Fetch == nil || *cfg.Defaults.Fetch != false {
		t.Error("Defaults.Fetch should be false")
	}
}

func TestLoadFile_MissingFile(t *testing.T) {
	_, err := loadFile("/nonexistent/config.json")
	if err == nil {
		t.Error("loadFile() should return error for missing file")
	}
}

func TestLoadFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{not json}`), 0o644)

	_, err := loadFile(path)
	if err == nil {
		t.Error("loadFile() should return error for invalid JSON")
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
			Fetch:           boolPtr(false),
			AutoSetupRemote: boolPtr(true),
		},
	}

	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile() error: %v", err)
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

func TestLoad_ThreeTierMerge(t *testing.T) {
	// Set up directory structure:
	//   globalDir/.config/willow/config.json  (global)
	//   wtRoot/.willow/config.json            (shared)
	//   bareDir/willow.json                   (local)

	tmp := t.TempDir()

	// We can't override GlobalConfigPath() easily since it uses os.UserHomeDir.
	// Instead, test the merge behavior by calling loadFile + merge directly,
	// which is what Load does internally.

	// Simulate global: baseBranch=main, fetch=true, setup=[npm install]
	globalDir := filepath.Join(tmp, "global")
	os.MkdirAll(globalDir, 0o755)
	os.WriteFile(filepath.Join(globalDir, "config.json"), []byte(`{
		"baseBranch": "main",
		"setup": ["npm install"],
		"defaults": {"fetch": true}
	}`), 0o644)

	// Simulate shared: branchPrefix=team, setup=[yarn install] (overrides global)
	sharedDir := filepath.Join(tmp, "shared")
	os.MkdirAll(sharedDir, 0o755)
	os.WriteFile(filepath.Join(sharedDir, "config.json"), []byte(`{
		"branchPrefix": "team",
		"setup": ["yarn install"]
	}`), 0o644)

	// Simulate local: baseBranch=develop, fetch=false (overrides both)
	localDir := filepath.Join(tmp, "local")
	os.MkdirAll(localDir, 0o755)
	os.WriteFile(filepath.Join(localDir, "config.json"), []byte(`{
		"baseBranch": "develop",
		"defaults": {"fetch": false}
	}`), 0o644)

	cfg := DefaultConfig()

	global, _ := loadFile(filepath.Join(globalDir, "config.json"))
	merge(cfg, global)

	shared, _ := loadFile(filepath.Join(sharedDir, "config.json"))
	merge(cfg, shared)

	local, _ := loadFile(filepath.Join(localDir, "config.json"))
	merge(cfg, local)

	// baseBranch: global=main, local=develop → develop wins
	if cfg.BaseBranch != "develop" {
		t.Errorf("BaseBranch = %q, want %q", cfg.BaseBranch, "develop")
	}
	// branchPrefix: only shared sets it → team
	if cfg.BranchPrefix != "team" {
		t.Errorf("BranchPrefix = %q, want %q", cfg.BranchPrefix, "team")
	}
	// setup: global=[npm install], shared=[yarn install] → yarn install wins
	if len(cfg.Setup) != 1 || cfg.Setup[0] != "yarn install" {
		t.Errorf("Setup = %v, want [yarn install]", cfg.Setup)
	}
	// fetch: default=true, global=true, local=false → false wins
	if *cfg.Defaults.Fetch != false {
		t.Error("Defaults.Fetch should be false (local override)")
	}
	// autoSetupRemote: only default sets it → true
	if *cfg.Defaults.AutoSetupRemote != true {
		t.Error("Defaults.AutoSetupRemote should remain true (no overrides)")
	}
}

func TestLoad_NoConfigFiles(t *testing.T) {
	// When no config files exist, Load should return defaults
	cfg := Load("/nonexistent/bare", "/nonexistent/worktree")

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

func TestLoad_EmptyBareAndWorktree(t *testing.T) {
	// When bare and worktree are empty strings, should still return defaults
	cfg := Load("", "")

	if *cfg.Defaults.Fetch != true {
		t.Error("Defaults.Fetch should be true")
	}
}
