package cli

import (
	"context"
	"fmt"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/urfave/cli/v3"
)

func setupCmd() *cli.Command {
	return &cli.Command{
		Name:  "cc-setup",
		Usage: "Install Claude Code hooks for status tracking",
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			u := flags.NewUI()

			if claude.IsInstalled() {
				u.Info("Claude Code hooks are already installed.")
				return nil
			}

			u.Info("Installing Claude Code status hooks...")

			if err := claude.Install(); err != nil {
				return err
			}

			u.Success("Installed Claude Code hooks")
			u.Info(fmt.Sprintf("  hook:   %s", u.Dim(claude.HookScriptPath())))
			u.Info(fmt.Sprintf("  status: %s", u.Dim(claude.StatusDir())))
			u.Info("")
			u.Info("Claude Code will now report agent status for willow-managed worktrees.")
			u.Info("Use 'ww status' or 'ww ls' to see agent status.")
			return nil
		},
	}
}
