package cli

import (
	"context"

	"github.com/iamrajjoshi/willow/internal/focus"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/urfave/cli/v3"
)

// focusCmd is the hidden subcommand a clickable desktop notification invokes to
// bring the triggering agent's terminal session to the foreground. It's wired
// into the notification's click action by the hook, not run by users directly.
func focusCmd() *cli.Command {
	return &cli.Command{
		Name:   "focus",
		Usage:  "Bring an agent session to the foreground (internal, invoked by notification clicks)",
		Hidden: true,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "session", Usage: "Session name (repo/worktree)", Required: true},
			&cli.StringFlag{Name: "tmux-socket", Usage: "tmux socket path; empty if the session isn't in tmux"},
			&cli.StringFlag{Name: "term-bundle", Usage: "Host terminal bundle id to activate"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.focus")()
			return focus.Focus(focus.Target{
				Session:    cmd.String("session"),
				TmuxSocket: cmd.String("tmux-socket"),
				TermBundle: cmd.String("term-bundle"),
			})
		},
	}
}
