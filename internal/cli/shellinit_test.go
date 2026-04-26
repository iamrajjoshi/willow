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

func TestDetectShell(t *testing.T) {
	tests := []struct {
		shell string
		want  string
	}{
		{"/bin/bash", "bash"},
		{"/opt/homebrew/bin/zsh", "zsh"},
		{"/usr/local/bin/fish", "fish"},
		{"/bin/nu", "bash"},
		{"", "bash"},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			t.Setenv("SHELL", tt.shell)
			if got := detectShell(); got != tt.want {
				t.Fatalf("detectShell() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShellInitCommandPrintsDetectedShellScript(t *testing.T) {
	tests := []struct {
		name  string
		shell string
		want  string
	}{
		{"bash", "/bin/bash", "ww()"},
		{"zsh", "/bin/zsh", "ww()"},
		{"fish", "/usr/local/bin/fish", "function ww"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("SHELL", tt.shell)

			out, err := captureStdout(t, func() error {
				return runApp("shell-init", "--tab-title")
			})
			if err != nil {
				t.Fatalf("shell-init failed: %v", err)
			}
			if !strings.Contains(out, tt.want) {
				t.Fatalf("shell-init output missing %q:\n%s", tt.want, out)
			}
			if !strings.Contains(out, "WILLOW_WORKTREES_DIR") {
				t.Fatalf("shell-init output missing worktrees dir:\n%s", out)
			}
			if !strings.Contains(out, "tab") && !strings.Contains(out, "title") && !strings.Contains(out, "precmd") && !strings.Contains(out, "fish_prompt") {
				t.Fatalf("shell-init --tab-title output missing tab-title integration:\n%s", out)
			}
		})
	}
}

func TestShellInitCommandDefaultsToBash(t *testing.T) {
	t.Setenv("SHELL", "")
	out, err := captureStdout(t, func() error {
		return runApp("shell-init")
	})
	if err != nil {
		t.Fatalf("shell-init failed: %v", err)
	}
	if !strings.Contains(out, "ww()") {
		t.Fatalf("default shell-init output should be bash script:\n%s", out)
	}
}
