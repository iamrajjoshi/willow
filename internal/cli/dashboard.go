package cli

import (
	"context"
	"time"

	"github.com/iamrajjoshi/willow/internal/dashboard"
	"github.com/iamrajjoshi/willow/internal/trace"
	"github.com/urfave/cli/v3"
)

func dashboardCmd() *cli.Command {
	return &cli.Command{
		Name:    "dashboard",
		Aliases: []string{"dash", "d"},
		Usage:   "Live overview of all worktrees across repos",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "interval",
				Aliases: []string{"i"},
				Usage:   "Refresh interval in seconds",
				Value:   2,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			defer trace.Span(ctx, "cli.dashboard")()
			interval := time.Duration(cmd.Int("interval")) * time.Second
			return dashboard.Run(ctx, dashboard.Config{
				RefreshInterval: interval,
			})
		},
	}
}
