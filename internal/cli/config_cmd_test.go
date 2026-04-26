package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/config"
)

func TestConfigCmd_Structure(t *testing.T) {
	cmd := configCmd()

	if cmd.Name != "config" {
		t.Errorf("Name = %q, want %q", cmd.Name, "config")
	}

	if len(cmd.Commands) != 3 {
		t.Fatalf("expected 3 subcommands, got %d", len(cmd.Commands))
	}

	names := map[string]bool{}
	for _, sub := range cmd.Commands {
		names[sub.Name] = true
	}
	for _, want := range []string{"show", "edit", "init"} {
		if !names[want] {
			t.Errorf("missing subcommand %q", want)
		}
	}
}

func TestConfigShowJSONUsesGlobalConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgPath := config.GlobalConfigPath()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"branchPrefix":"raj","defaults":{"fetch":false}}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runApp("config", "show", "--json")
	})
	if err != nil {
		t.Fatalf("config show --json failed: %v", err)
	}

	var got config.Config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("config show output is not JSON: %v\n%s", err, out)
	}
	if got.BranchPrefix != "raj" {
		t.Fatalf("BranchPrefix = %q, want raj", got.BranchPrefix)
	}
	if got.Defaults.Fetch == nil || *got.Defaults.Fetch {
		t.Fatalf("Defaults.Fetch = %v, want false", got.Defaults.Fetch)
	}
	if got.BaseDir != filepath.Join(home, ".willow") {
		t.Fatalf("BaseDir = %q, want default willow home", got.BaseDir)
	}
}

func TestConfigShowPrintsSourceAnnotationsAndWarnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgPath := config.GlobalConfigPath()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"tmux":{"panes":[{"command":"echo hi"}]}}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runApp("config", "show")
	})
	if err != nil {
		t.Fatalf("config show failed: %v", err)
	}
	for _, want := range []string{"baseDir:", "# default", "tmux.panes:", "# global", "tmux.panes configured"} {
		if !strings.Contains(out, want) {
			t.Fatalf("config show output missing %q:\n%s", want, out)
		}
	}
}

func TestConfigEditCreatesFileAndRunsEditor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	logPath := filepath.Join(home, "editor.log")
	editorPath := filepath.Join(home, "editor")
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> " + logPath + "\n"
	if err := os.WriteFile(editorPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write editor: %v", err)
	}
	t.Setenv("VISUAL", editorPath)

	if err := runApp("config", "edit"); err != nil {
		t.Fatalf("config edit failed: %v", err)
	}

	cfgPath := config.GlobalConfigPath()
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("expected config file to be created: %v", err)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read editor log: %v", err)
	}
	if strings.TrimSpace(string(logData)) != cfgPath {
		t.Fatalf("editor invoked with %q, want %q", strings.TrimSpace(string(logData)), cfgPath)
	}
}

func TestConfigInitCreatesGlobalConfigAndRejectsExisting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdin: %v", err)
	}
	if _, err := w.WriteString("n\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	out, err := captureStdout(t, func() error {
		return runApp("config", "init")
	})
	if err != nil {
		t.Fatalf("config init failed: %v", err)
	}
	if !strings.Contains(out, "Created config") {
		t.Fatalf("config init output missing success:\n%s", out)
	}
	cfg, err := config.LoadFile(config.GlobalConfigPath())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Telemetry == nil || *cfg.Telemetry {
		t.Fatalf("Telemetry = %v, want false", cfg.Telemetry)
	}

	os.Stdin = origStdin
	err = runApp("config", "init")
	if err == nil {
		t.Fatal("config init should reject existing config")
	}
	if !strings.Contains(err.Error(), "config already exists") {
		t.Fatalf("error = %v, want already exists", err)
	}
}

func TestConfigInitForceOverwritesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeGlobalConfigFile(t, `{"branchPrefix":"old","telemetry":false}`)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdin: %v", err)
	}
	if _, err := w.WriteString("y\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	out, err := captureStdout(t, func() error {
		return runApp("config", "init", "--force")
	})
	if err != nil {
		t.Fatalf("config init --force failed: %v", err)
	}
	if !strings.Contains(out, "Created config") {
		t.Fatalf("config init --force output missing success:\n%s", out)
	}
	cfg, err := config.LoadFile(config.GlobalConfigPath())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.BranchPrefix != "" {
		t.Fatalf("BranchPrefix = %q, want overwritten default", cfg.BranchPrefix)
	}
	if cfg.Telemetry == nil || !*cfg.Telemetry {
		t.Fatalf("Telemetry = %v, want true", cfg.Telemetry)
	}
}

func TestFieldSource_String(t *testing.T) {
	tests := []struct {
		name    string
		local   string
		global  string
		def     string
		wantSrc string
	}{
		{"local set", "val", "", "", "local"},
		{"global set", "", "val", "", "global"},
		{"default set", "", "", "val", "default"},
		{"local wins over global", "local", "global", "def", "local"},
		{"global wins over default", "", "global", "def", "global"},
		{"all empty", "", "", "", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldSource(tt.local, tt.global, tt.def)
			if got != tt.wantSrc {
				t.Errorf("fieldSource(%q, %q, %q) = %q, want %q",
					tt.local, tt.global, tt.def, got, tt.wantSrc)
			}
		})
	}
}

func TestFieldSource_Int(t *testing.T) {
	tests := []struct {
		name    string
		local   int
		global  int
		def     int
		wantSrc string
	}{
		{"local set", 5, 0, 0, "local"},
		{"global set", 0, 3, 0, "global"},
		{"all zero", 0, 0, 0, "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldSource(tt.local, tt.global, tt.def)
			if got != tt.wantSrc {
				t.Errorf("fieldSource(%d, %d, %d) = %q, want %q",
					tt.local, tt.global, tt.def, got, tt.wantSrc)
			}
		})
	}
}

func TestFieldSourceBoolPtr(t *testing.T) {
	tr := config.BoolPtr(true)
	fa := config.BoolPtr(false)

	tests := []struct {
		name    string
		local   *bool
		global  *bool
		def     *bool
		wantSrc string
	}{
		{"local set", tr, nil, nil, "local"},
		{"global set", nil, fa, nil, "global"},
		{"default set", nil, nil, tr, "default"},
		{"local wins", fa, tr, tr, "local"},
		{"all nil", nil, nil, nil, "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldSourceBoolPtr(tt.local, tt.global, tt.def)
			if got != tt.wantSrc {
				t.Errorf("fieldSourceBoolPtr = %q, want %q", got, tt.wantSrc)
			}
		})
	}
}

func TestFieldSourceSlice(t *testing.T) {
	tests := []struct {
		name    string
		local   []string
		global  []string
		def     []string
		wantSrc string
	}{
		{"local set", []string{"a"}, nil, nil, "local"},
		{"global set", nil, []string{"b"}, nil, "global"},
		{"all nil", nil, nil, nil, "default"},
		{"local empty slice", []string{}, nil, nil, "local"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldSourceSlice(tt.local, tt.global, tt.def)
			if got != tt.wantSrc {
				t.Errorf("fieldSourceSlice = %q, want %q", got, tt.wantSrc)
			}
		})
	}
}

func TestBaseDirSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Run("env wins", func(t *testing.T) {
		t.Setenv("WILLOW_BASE_DIR", filepath.Join(home, "env-base"))
		if got := baseDirSource(filepath.Join(home, "global-base")); got != "env" {
			t.Errorf("baseDirSource() = %q, want %q", got, "env")
		}
	})

	t.Run("global when env unset", func(t *testing.T) {
		t.Setenv("WILLOW_BASE_DIR", "")
		if got := baseDirSource(filepath.Join(home, "global-base")); got != "global" {
			t.Errorf("baseDirSource() = %q, want %q", got, "global")
		}
	})

	t.Run("default when unset", func(t *testing.T) {
		t.Setenv("WILLOW_BASE_DIR", "")
		if got := baseDirSource(""); got != "default" {
			t.Errorf("baseDirSource() = %q, want %q", got, "default")
		}
	})
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{"string", "hello", `"hello"`},
		{"empty string", "", `""`},
		{"nil bool ptr", (*bool)(nil), "<nil>"},
		{"true bool ptr", config.BoolPtr(true), "true"},
		{"false bool ptr", config.BoolPtr(false), "false"},
		{"nil slice", ([]string)(nil), "[]"},
		{"empty slice", []string{}, "[]"},
		{"string slice", []string{"a", "b"}, "[a b]"},
		{"int", 42, "42"},
		{"zero int", 0, "0"},
		{"nil panes", ([]config.PaneConfig)(nil), "[]"},
		{"panes", []config.PaneConfig{{}, {Command: "x"}}, "[2 panes]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatValue(tt.val)
			if got != tt.want {
				t.Errorf("formatValue(%v) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}
