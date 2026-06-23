package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/iamrajjoshi/willow/internal/agent/harness"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errors"
	"github.com/iamrajjoshi/willow/internal/log"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/iamrajjoshi/willow/internal/ui"
	"github.com/urfave/cli/v3"
)

func dispatchCmd() *cli.Command {
	return &cli.Command{
		Name:  "dispatch",
		Usage: "Create a worktree and launch an agent harness with a prompt",
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
				Usage: "Run the agent harness with its full-access permissions flag",
			},
			&cli.StringFlag{
				Name:  "agent",
				Usage: "Agent harness to launch (claude or codex; default from agent.default)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.dispatch")()
			flags := parseFlags(cmd)
			g := flags.NewGit()
			u := flags.NewUI()

			prompt := cmd.StringArg("prompt")
			if prompt == "" {
				return errors.Userf("prompt is required\n\nUsage: ww dispatch <prompt> [flags]")
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
			cfg := config.Load(bareDir)
			agentID := cmd.String("agent")
			if agentID == "" {
				agentID = harness.DefaultID(cfg)
			}
			h, err := harness.MustGet(agentID)
			if err != nil {
				return err
			}

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

			meta := map[string]string{"prompt": truncatePrompt(prompt), "agent": h.ID()}
			_ = log.Append(log.Event{Action: "dispatch", Repo: repoName, Branch: branch, Metadata: meta})

			return dispatchForeground(u, wtPath, branch, prompt, h, cfg, cmd.Bool("yolo"))
		},
	}
}

func dispatchForeground(u *ui.UI, wtPath, branch, prompt string, h harness.Harness, cfg *config.Config, yolo bool) error {
	launch := h.BuildLaunch(harness.LaunchOptions{
		Prompt:    prompt,
		Yolo:      yolo,
		Overrides: harness.OverridesFor(cfg, h.ID()),
	})
	if _, err := exec.LookPath(launch.Command); err != nil {
		return errors.Userf("%q CLI not found — install %s first", launch.Command, h.DisplayName())
	}

	u.Success(fmt.Sprintf("Dispatched %s on %s (foreground)", h.DisplayName(), u.Bold(branch)))
	u.Info(fmt.Sprintf("  path: %s", u.Dim(wtPath)))
	u.Info("")

	cmd := exec.Command(launch.Command, launch.Args...)
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
