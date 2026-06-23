package cli

import (
	"context"
	"os"

	"github.com/iamrajjoshi/willow/internal/agent"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/urfave/cli/v3"
)

// hookCmd is the hidden subcommand agent harnesses invoke directly for every
// configured hook event. It reads the JSON payload from stdin, writes session
// status files, and fires desktop notifications inline.
func hookCmd() *cli.Command {
	return &cli.Command{
		Name:   "hook",
		Usage:  "Agent status hook (internal, registered by agent setup)",
		Hidden: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "harness",
				Usage: "Agent harness that produced the hook payload",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.hook")()
			return agent.HandleHook(os.Stdin, cmd.String("harness"))
		},
	}
}
