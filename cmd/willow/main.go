package main

import (
	"context"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/iamrajjoshi/willow/internal/cli"
	"github.com/iamrajjoshi/willow/internal/telemetry"
	"github.com/iamrajjoshi/willow/internal/ui"
)

func main() {
	os.Exit(run())
}

func run() int {
	cleanup := telemetry.Init(cli.Version())
	defer cleanup()

	defer func() {
		if r := recover(); r != nil {
			sentry.CurrentHub().Recover(r)
			sentry.Flush(2 * time.Second)
			os.Exit(2)
		}
	}()

	root := cli.NewApp()
	cmdName := telemetry.ResolveCommandName(os.Args)
	ctx, finish := telemetry.StartCommand(context.Background(), cmdName)

	err := root.Run(ctx, os.Args)
	finish(err)

	if err != nil {
		u := &ui.UI{}
		u.Errorf("%v", err)
		return 1
	}
	return 0
}
