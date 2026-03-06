package fzf

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type config struct {
	header  string
	ansi    bool
	reverse bool
}

type Option func(*config)

func WithHeader(h string) Option {
	return func(c *config) { c.header = h }
}

func WithAnsi() Option {
	return func(c *config) { c.ansi = true }
}

func WithReverse() Option {
	return func(c *config) { c.reverse = true }
}

// Run launches fzf with the given lines and returns the selected line.
// Returns empty string and nil error if user cancelled (Esc/Ctrl-C).
func Run(lines []string, opts ...Option) (string, error) {
	if _, err := exec.LookPath("fzf"); err != nil {
		return "", fmt.Errorf("fzf is required but not found in PATH\n\nInstall it: https://github.com/junegunn/fzf#installation")
	}

	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}

	args := []string{"--no-multi"}
	if cfg.ansi {
		args = append(args, "--ansi")
	}
	if cfg.reverse {
		args = append(args, "--reverse")
	}
	if cfg.header != "" {
		args = append(args, "--header", cfg.header)
	}

	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// fzf returns 130 on Ctrl-C, 1 on no match
			if exitErr.ExitCode() == 130 || exitErr.ExitCode() == 1 {
				return "", nil
			}
		}
		return "", fmt.Errorf("fzf failed: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}
