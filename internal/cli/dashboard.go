package cli

import (
	"context"
	"time"

	"github.com/iamrajjoshi/willow/internal/dashboard"
	"github.com/urfave/cli/v3"
)

func dashboardCmd() *cli.Command {
	return &cli.Command{
		Name:    "dashboard",
		Aliases: []string{"dash", "d"},
		Usage:   "Live overview of all Claude Code sessions across repos",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "interval",
				Aliases: []string{"i"},
				Usage:   "Refresh interval in seconds",
				Value:   2,
			},
			&cli.BoolFlag{
				Name:  "no-timeline",
				Usage: "Hide the timeline column",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			interval := time.Duration(cmd.Int("interval")) * time.Second
			return dashboard.Run(ctx, dashboard.Config{
				RefreshInterval: interval,
				ShowTimeline:    !cmd.Bool("no-timeline"),
			})
		},
	}
}
