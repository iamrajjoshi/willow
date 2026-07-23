package cli

import (
	"context"

	"github.com/iamrajjoshi/willow/internal/focus"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/urfave/cli/v3"
)

// focusCmd is the hidden subcommand a notification click invokes to foreground the session.
func focusCmd() *cli.Command {
	return &cli.Command{
		Name:   "focus",
		Usage:  "Bring an agent session to the foreground (internal, invoked by notification clicks)",
		Hidden: true,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "session", Usage: "Session name (repo/worktree)", Required: true},
			&cli.StringFlag{Name: "tmux-path", Usage: "Absolute path to tmux captured by the hook"},
			&cli.StringFlag{Name: "tmux-socket", Usage: "tmux socket path; empty if the session isn't in tmux"},
			&cli.StringFlag{Name: "tmux-pane", Usage: "Agent tmux pane id"},
			&cli.StringFlag{Name: "tmux-client", Usage: "tmux client that originated the notification"},
			&cli.StringFlag{Name: "term-bundle", Usage: "Host terminal bundle id to activate"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.focus")()
			return focus.Focus(focus.Target{
				Session:    cmd.String("session"),
				TmuxPath:   cmd.String("tmux-path"),
				TmuxSocket: cmd.String("tmux-socket"),
				TmuxPane:   cmd.String("tmux-pane"),
				TmuxClient: cmd.String("tmux-client"),
				TermBundle: cmd.String("term-bundle"),
			})
		},
	}
}
