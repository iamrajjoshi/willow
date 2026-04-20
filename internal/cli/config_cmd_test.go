package cli

import (
	"path/filepath"
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
