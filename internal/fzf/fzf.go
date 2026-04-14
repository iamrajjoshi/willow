package fzf

import (
	"fmt"
	"strings"

	fzflib "github.com/junegunn/fzf/src"
)

type config struct {
	header     string
	ansi       bool
	reverse    bool
	noSort     bool
	cycle      bool
	preview    string
	previewWin string
	expectKeys string
	printQuery bool
	query      string
	bindings   []string
	delimiter  string
	nth        string
}

func defaults() *config {
	return &config{cycle: true}
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

func WithNoSort() Option {
	return func(c *config) { c.noSort = true }
}

func WithPreview(cmd, window string) Option {
	return func(c *config) {
		c.preview = cmd
		c.previewWin = window
	}
}

func WithExpectKeys(keys ...string) Option {
	return func(c *config) { c.expectKeys = strings.Join(keys, ",") }
}

func WithPrintQuery() Option {
	return func(c *config) { c.printQuery = true }
}

func WithCycle() Option {
	return func(c *config) { c.cycle = true }
}

func WithQuery(q string) Option {
	return func(c *config) { c.query = q }
}

func WithBind(bindings ...string) Option {
	return func(c *config) { c.bindings = append(c.bindings, bindings...) }
}

func WithDelimiter(d string) Option {
	return func(c *config) { c.delimiter = d }
}

func WithNth(n string) Option {
	return func(c *config) { c.nth = n }
}

func buildArgs(cfg *config) []string {
	var args []string
	if cfg.ansi {
		args = append(args, "--ansi")
	}
	if cfg.reverse {
		args = append(args, "--reverse")
	}
	if cfg.noSort {
		args = append(args, "--no-sort")
	}
	if cfg.cycle {
		args = append(args, "--cycle")
	}
	if cfg.header != "" {
		args = append(args, "--header", cfg.header)
	}
	if cfg.preview != "" {
		args = append(args, "--preview", cfg.preview)
	}
	if cfg.previewWin != "" {
		args = append(args, "--preview-window", cfg.previewWin)
	}
	if cfg.expectKeys != "" {
		args = append(args, "--expect", cfg.expectKeys)
	}
	if cfg.printQuery {
		args = append(args, "--print-query")
	}
	if cfg.query != "" {
		args = append(args, "--query", cfg.query)
	}
	for _, b := range cfg.bindings {
		args = append(args, "--bind", b)
	}
	if cfg.delimiter != "" {
		args = append(args, "--delimiter", cfg.delimiter)
	}
	if cfg.nth != "" {
		args = append(args, "--nth", cfg.nth)
	}
	return args
}

func runFzf(lines []string, extraArgs []string, cfg *config) ([]string, int, error) {
	args := append(extraArgs, buildArgs(cfg)...)

	opts, err := fzflib.ParseOptions(true, args)
	if err != nil {
		return nil, 0, fmt.Errorf("fzf: %w", err)
	}

	inputChan := make(chan string, len(lines))
	for _, line := range lines {
		inputChan <- line
	}
	close(inputChan)
	opts.Input = inputChan

	outputChan := make(chan string, 128)
	opts.Output = outputChan

	// Drain output channel concurrently to prevent deadlock
	var results []string
	done := make(chan struct{})
	go func() {
		for s := range outputChan {
			results = append(results, s)
		}
		close(done)
	}()

	code, err := fzflib.Run(opts)
	close(outputChan)
	<-done

	return results, code, err
}

// Run launches fzf with the given lines and returns the selected line.
// Returns empty string and nil error if user cancelled (Esc/Ctrl-C).
func Run(lines []string, opts ...Option) (string, error) {
	cfg := defaults()
	for _, o := range opts {
		o(cfg)
	}

	results, code, err := runFzf(lines, []string{"--no-multi"}, cfg)
	if code == fzflib.ExitInterrupt || code == fzflib.ExitNoMatch {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("fzf failed: %w", err)
	}
	if len(results) == 0 {
		return "", nil
	}
	return results[0], nil
}

type ExpectResult struct {
	Query     string
	Key       string
	Selection string
}

// RunExpect launches fzf with --print-query and --expect, returning structured output.
// Returns nil, nil on cancel (Esc/Ctrl-C).
func RunExpect(lines []string, opts ...Option) (*ExpectResult, error) {
	cfg := defaults()
	for _, o := range opts {
		o(cfg)
	}

	results, code, err := runFzf(lines, []string{"--no-multi"}, cfg)
	if code == fzflib.ExitInterrupt {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fzf failed: %w", err)
	}

	// With --print-query + --expect, output is:
	// line 0: query text
	// line 1: key pressed (from --expect)
	// line 2: selected line
	result := &ExpectResult{}
	if len(results) > 0 {
		result.Query = results[0]
	}
	if len(results) > 1 {
		result.Key = results[1]
	}
	if len(results) > 2 {
		result.Selection = results[2]
	}
	return result, nil
}

// RunMulti launches fzf with multi-select enabled (TAB to toggle, Ctrl-A to select all).
// Returns nil, nil on cancel (Esc/Ctrl-C).
func RunMulti(lines []string, opts ...Option) ([]string, error) {
	cfg := defaults()
	for _, o := range opts {
		o(cfg)
	}

	results, code, err := runFzf(lines, []string{"--multi", "--bind=ctrl-a:select-all"}, cfg)
	if code == fzflib.ExitInterrupt || code == fzflib.ExitNoMatch {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fzf failed: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results, nil
}
