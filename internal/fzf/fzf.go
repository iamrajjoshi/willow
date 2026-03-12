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

func buildArgs(cfg *config) []string {
	var args []string
	if cfg.ansi {
		args = append(args, "--ansi")
	}
	if cfg.reverse {
		args = append(args, "--reverse")
	}
	if cfg.header != "" {
		args = append(args, "--header", cfg.header)
	}
	return args
}

func requireFzf() error {
	if _, err := exec.LookPath("fzf"); err != nil {
		return fmt.Errorf("fzf is required but not found in PATH\n\nInstall it: https://github.com/junegunn/fzf#installation")
	}
	return nil
}

func isCancelled(err error) bool {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode() == 130 || exitErr.ExitCode() == 1
	}
	return false
}

// Run launches fzf with the given lines and returns the selected line.
// Returns empty string and nil error if user cancelled (Esc/Ctrl-C).
func Run(lines []string, opts ...Option) (string, error) {
	if err := requireFzf(); err != nil {
		return "", err
	}

	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}

	args := append([]string{"--no-multi"}, buildArgs(cfg)...)

	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		if isCancelled(err) {
			return "", nil
		}
		return "", fmt.Errorf("fzf failed: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// RunMulti launches fzf with multi-select enabled (TAB to toggle, Ctrl-A to select all).
// Returns nil, nil on cancel (Esc/Ctrl-C).
func RunMulti(lines []string, opts ...Option) ([]string, error) {
	if err := requireFzf(); err != nil {
		return nil, err
	}

	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}

	args := append([]string{"--multi", "--bind=ctrl-a:select-all"}, buildArgs(cfg)...)

	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		if isCancelled(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("fzf failed: %w", err)
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}

	return strings.Split(raw, "\n"), nil
}
