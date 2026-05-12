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

func TestShellInitScriptsHandlePromoteCd(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name:   "bash",
			script: renderBashInitScript(),
			want:   `command willow promote "${@:2}" --cd`,
		},
		{
			name:   "zsh",
			script: renderZshInitScript(),
			want:   `command willow promote "${@:2}" --cd`,
		},
		{
			name:   "fish",
			script: renderFishInitScript(),
			want:   `command willow promote $argv[2..] --cd`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.script, tt.want) {
				t.Fatalf("script missing promote cd hook %q:\n%s", tt.want, tt.script)
			}
		})
	}
}

func TestShellInitCompletionsBypassWwFunction(t *testing.T) {
	// The ww shell function captures stdout from `ww sw` / `ww co` / etc. and
	// passes it to `cd`. If the completion script invokes the function (rather
	// than the binary), the completion list is fed to `cd` instead of the
	// completion engine. Make sure each shell's completion section calls the
	// willow binary directly — checking only the completion block, since the
	// rest of the script also contains `command willow` for unrelated reasons.
	tests := []struct {
		name    string
		script  string
		marker  string // line that begins the completion section
		want    []string
		notWant []string
	}{
		{
			name:   "bash",
			script: renderBashInitScript(),
			marker: "__willow_bash_autocomplete()",
			want:   []string{"command willow"},
			notWant: []string{
				`requestComp="${words[*]} ${cur} --generate-shell-completion"`,
				`requestComp="${words[*]} --generate-shell-completion"`,
			},
		},
		{
			name:   "zsh",
			script: renderZshInitScript(),
			marker: "_willow()",
			want:   []string{"command willow"},
			notWant: []string{
				`${words[@]:0:#words[@]-1} ${current} --generate-shell-completion`,
				`${words[@]:0:#words[@]-1} --generate-shell-completion`,
			},
		},
		{
			name:   "fish",
			script: renderFishInitScript(),
			marker: "function __fish_willow_complete",
			want:   []string{"command willow"},
			notWant: []string{
				`$tokens $cur --generate-shell-completion`,
				`$tokens --generate-shell-completion 2>/dev/null`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := strings.Index(tt.script, tt.marker)
			if idx < 0 {
				t.Fatalf("completion marker %q not found in script", tt.marker)
			}
			section := tt.script[idx:]
			for _, w := range tt.want {
				if !strings.Contains(section, w) {
					t.Fatalf("completion section must contain %q:\n%s", w, section)
				}
			}
			for _, bad := range tt.notWant {
				if strings.Contains(section, bad) {
					t.Fatalf("completion section must not invoke the ww shell function via %q:\n%s", bad, section)
				}
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
