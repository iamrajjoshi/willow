package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/iamrajjoshi/willow/internal/agent"
	"github.com/iamrajjoshi/willow/internal/agent/harness"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/urfave/cli/v3"
)

func setupCmd() *cli.Command {
	return &cli.Command{
		Name:  "cc-setup",
		Usage: "Install Claude Code hooks for status tracking",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.cc-setup")()
			return runAgentSetup(cmd, []string{harness.ClaudeID})
		},
	}
}

func codexSetupCmd() *cli.Command {
	return &cli.Command{
		Name:  "codex-setup",
		Usage: "Install Codex CLI hooks for status tracking",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.codex-setup")()
			return runAgentSetup(cmd, []string{harness.CodexID})
		},
	}
}

func agentCmd() *cli.Command {
	return &cli.Command{
		Name:  "agent",
		Usage: "Manage agent harness integrations",
		Commands: []*cli.Command{
			{
				Name:  "setup",
				Usage: "Install agent hooks for claude, codex, or all",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "harness",
						UsageText: "[claude|codex|all]",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					defer trace.Span(ctx, "cli.agent.setup")()
					target := strings.TrimSpace(cmd.StringArg("harness"))
					if target == "" {
						target = "all"
					}
					ids, err := setupTargets(target)
					if err != nil {
						return err
					}
					return runAgentSetup(cmd, ids)
				},
			},
		},
	}
}

func setupTargets(target string) ([]string, error) {
	switch harness.NormalizeID(target) {
	case "all":
		return []string{harness.ClaudeID, harness.CodexID}, nil
	case harness.ClaudeID:
		return []string{harness.ClaudeID}, nil
	case harness.CodexID:
		return []string{harness.CodexID}, nil
	default:
		return nil, fmt.Errorf("unknown agent harness %q (expected claude, codex, or all)", target)
	}
}

func runAgentSetup(cmd *cli.Command, ids []string) error {
	flags := parseFlags(cmd)
	u := flags.NewUI()

	for _, id := range ids {
		h, err := harness.MustGet(id)
		if err != nil {
			return err
		}
		changed, err := agent.InstallHarness(id)
		if err != nil {
			return err
		}
		hookCmd, err := agent.HookCommandForHarness(id)
		if err != nil {
			return err
		}

		if changed {
			u.Success(fmt.Sprintf("Installed %s hooks", h.DisplayName()))
		} else {
			u.Success(fmt.Sprintf("%s hooks up to date", h.DisplayName()))
		}
		u.Info(fmt.Sprintf("  hook:   %s", u.Dim(hookCmd)))
		u.Info(fmt.Sprintf("  status: %s", u.Dim(agent.StatusDir())))
		if hint := h.DocsHint(); hint != "" {
			u.Info(fmt.Sprintf("  note:   %s", hint))
		}
	}

	u.Info("")
	u.Info("Agent sessions will now report status for willow-managed worktrees.")
	u.Info("Use 'ww status' or 'ww ls' to see agent status.")
	u.Info("Desktop notifications are enabled by default for agent finish/input events.")
	u.Info("Set notify.desktop=false to disable, or notify.command to customize.")

	cfg := config.Load("")
	if cfg.Telemetry == nil {
		u.Info("")
		enabled := u.Confirm("Enable anonymous error telemetry (crash reports)?")
		cfg.Telemetry = config.BoolPtr(enabled)
		if err := config.Save(cfg, config.GlobalConfigPath()); err != nil {
			return fmt.Errorf("failed to save telemetry preference: %w", err)
		}
		if enabled {
			u.Success("Telemetry enabled")
		} else {
			u.Info("Telemetry disabled")
		}
	}

	return nil
}
