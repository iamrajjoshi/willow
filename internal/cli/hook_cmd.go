package cli

import (
	"context"
	"os"

	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/urfave/cli/v3"
)

// hookCmd is the hidden subcommand Claude Code invokes directly for every
// configured hook event. It reads the JSON payload from stdin, writes session
// status files, and fires desktop notifications inline.
func hookCmd() *cli.Command {
	return &cli.Command{
		Name:   "hook",
		Usage:  "Claude Code status hook (internal, registered by cc-setup)",
		Hidden: true,
		Action: func(ctx context.Context, _ *cli.Command) error {
			defer trace.Span(ctx, "cli.hook")()
			return claude.HandleHook(os.Stdin)
		},
	}
}
