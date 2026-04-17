package main

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/iamrajjoshi/willow/internal/cli"
	"github.com/iamrajjoshi/willow/internal/telemetry"
	"github.com/iamrajjoshi/willow/internal/trace"
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

	// urfave/cli hasn't parsed flags yet, so inspect argv/env directly. The
	// tracer only controls stderr output — Sentry spans are wired through
	// the hook registered by telemetry.Init and fire regardless of --trace.
	tr := trace.New(traceStderrRequested(os.Args))
	ctx = trace.WithTracer(ctx, tr)

	err := root.Run(ctx, os.Args)
	finish(err)

	if err != nil {
		u := &ui.UI{}
		u.Errorf("%v", err)
		return 1
	}
	return 0
}

func traceStderrRequested(args []string) bool {
	if v := strings.ToLower(os.Getenv("WILLOW_TRACE")); v == "1" || v == "true" || v == "on" {
		return true
	}
	for _, a := range args[1:] {
		if a == "--trace" || a == "-trace" {
			return true
		}
	}
	return false
}
