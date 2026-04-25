package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellInitScriptsUseConfiguredWorktreesDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WILLOW_BASE_DIR", filepath.Join(home, "custom willow"))

	want := filepath.Join(home, "custom willow", "worktrees")

	tests := []struct {
		name   string
		script string
		tab    string
		line   string
	}{
		{
			name:   "bash",
			script: renderBashInitScript(),
			tab:    renderBashTabTitle(),
			line:   fmt.Sprintf("export WILLOW_WORKTREES_DIR=%q", want),
		},
		{
			name:   "zsh",
			script: renderZshInitScript(),
			tab:    renderZshTabTitle(),
			line:   fmt.Sprintf("export WILLOW_WORKTREES_DIR=%q", want),
		},
		{
			name:   "fish",
			script: renderFishInitScript(),
			tab:    renderFishTabTitle(),
			line:   fmt.Sprintf("set -gx WILLOW_WORKTREES_DIR %q", want),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.script, tt.line) {
				t.Fatalf("script missing configured worktrees dir line %q:\n%s", tt.line, tt.script)
			}
			if !strings.Contains(tt.script, "$WILLOW_WORKTREES_DIR") {
				t.Fatalf("script should reference $WILLOW_WORKTREES_DIR:\n%s", tt.script)
			}
			if !strings.Contains(tt.tab, "$WILLOW_WORKTREES_DIR") {
				t.Fatalf("tab-title script should reference $WILLOW_WORKTREES_DIR:\n%s", tt.tab)
			}
		})
	}
}

func TestShellInitScriptsKeepParentFallbackForRm(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name:   "bash",
			script: renderBashInitScript(),
			want:   `cd "${cwd%/*}" 2>/dev/null || cd "$WILLOW_WORKTREES_DIR" 2>/dev/null || true`,
		},
		{
			name:   "zsh",
			script: renderZshInitScript(),
			want:   `cd "${cwd%/*}" 2>/dev/null || cd "$WILLOW_WORKTREES_DIR" 2>/dev/null || true`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.script, tt.want) {
				t.Fatalf("script missing rm fallback %q:\n%s", tt.want, tt.script)
			}
		})
	}
}

func TestShellInitScriptsHandleRenameCd(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name:   "bash",
			script: renderBashInitScript(),
			want:   `command willow rename "${@:2}" --cd`,
		},
		{
			name:   "zsh",
			script: renderZshInitScript(),
			want:   `command willow rename "${@:2}" --cd`,
		},
		{
			name:   "fish",
			script: renderFishInitScript(),
			want:   `command willow rename $argv[2..] --cd`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.script, tt.want) {
				t.Fatalf("script missing rename cd hook %q:\n%s", tt.want, tt.script)
			}
		})
	}
}
