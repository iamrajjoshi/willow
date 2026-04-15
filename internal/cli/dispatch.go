package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errs"
	"github.com/iamrajjoshi/willow/internal/log"
	"github.com/iamrajjoshi/willow/internal/ui"
	"github.com/urfave/cli/v3"
)

func dispatchCmd() *cli.Command {
	return &cli.Command{
		Name:  "dispatch",
		Usage: "Create a worktree and launch Claude Code with a prompt",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "prompt",
				UsageText: "<prompt>",
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "name",
				Usage: "Worktree/branch name (default: auto-generated from prompt)",
			},
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Target a willow-managed repo by name",
			},
			&cli.StringFlag{
				Name:    "base",
				Aliases: []string{"b"},
				Usage:   "Base branch to fork from",
			},
			&cli.BoolFlag{
				Name:  "no-fetch",
				Usage: "Skip fetching from remote",
			},
			&cli.BoolFlag{
				Name:  "yolo",
				Usage: "Run Claude with --dangerously-skip-permissions",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			g := flags.NewGit()
			u := flags.NewUI()

			prompt := cmd.StringArg("prompt")
			if prompt == "" {
				return errs.Userf("prompt is required\n\nUsage: ww dispatch <prompt> [flags]")
			}

			var bareDir string
			var err error
			if repoFlag := cmd.String("repo"); repoFlag != "" {
				bareDir, err = config.ResolveRepo(repoFlag)
				if err != nil {
					return err
				}
			} else {
				bareDir, err = requireWillowRepo(g)
				if err != nil {
					return err
				}
			}
			repoName := repoNameFromDir(bareDir)

			branch := cmd.String("name")
			if branch == "" {
				branch = "dispatch--" + slugify(prompt)
			}

			self, err := os.Executable()
			if err != nil {
				return fmt.Errorf("failed to find willow binary: %w", err)
			}

			args := []string{"new", "--cd", "--repo", repoName}
			if base := cmd.String("base"); base != "" {
				args = append(args, "--base", base)
			}
			if cmd.Bool("no-fetch") {
				args = append(args, "--no-fetch")
			}
			args = append(args, "--", branch)

			u.Info(fmt.Sprintf("Creating worktree %s...", u.Bold(branch)))
			newCmd := exec.Command(self, args...)
			newCmd.Stderr = os.Stderr
			out, err := newCmd.Output()
			if err != nil {
				return fmt.Errorf("failed to create worktree: %w", err)
			}
			wtPath := strings.TrimSpace(string(out))
			if wtPath == "" {
				return fmt.Errorf("no path returned from willow new")
			}

			promptFile := filepath.Join(wtPath, ".willow-prompt")
			if err := os.WriteFile(promptFile, []byte(prompt), 0o644); err != nil {
				return fmt.Errorf("failed to write prompt file: %w", err)
			}

			meta := map[string]string{"prompt": truncatePrompt(prompt)}
			_ = log.Append(log.Event{Action: "dispatch", Repo: repoName, Branch: branch, Metadata: meta})

			return dispatchForeground(u, wtPath, branch, prompt, cmd.Bool("yolo"))
		},
	}
}

func dispatchForeground(u *ui.UI, wtPath, branch, prompt string, yolo bool) error {
	if _, err := exec.LookPath("claude"); err != nil {
		return errs.Userf("'claude' CLI not found — install Claude Code first")
	}

	u.Success(fmt.Sprintf("Dispatched agent on %s (foreground)", u.Bold(branch)))
	u.Info(fmt.Sprintf("  path: %s", u.Dim(wtPath)))
	u.Info("")

	claudeArgs := "claude " + shellQuote(prompt)
	if yolo {
		claudeArgs += " --dangerously-skip-permissions"
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	cmd := exec.Command(shell, "-l", "-c", claudeArgs)
	cmd.Dir = wtPath
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var slugRe = regexp.MustCompile(`[^a-z0-9-]`)

func slugify(s string) string {
	words := strings.Fields(strings.ToLower(s))
	if len(words) > 5 {
		words = words[:5]
	}
	slug := strings.Join(words, "-")
	slug = slugRe.ReplaceAllString(slug, "")
	if len(slug) > 50 {
		slug = slug[:50]
	}
	return strings.TrimRight(slug, "-")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func truncatePrompt(s string) string {
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}
