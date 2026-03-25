package telemetry

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/getsentry/sentry-go/attribute"
	"github.com/iamrajjoshi/willow/internal/config"
)

var enabled bool

// Init initializes Sentry if telemetry is enabled.
// Returns a cleanup function that flushes buffered events.
func Init(version string) func() {
	noop := func() {}

	if isOptedOut() {
		return noop
	}

	env := "production"
	if version == "dev" || version == "" {
		env = "development"
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              "https://6f8ef87bf464515316f432ee9273a55c@o4509294838415360.ingest.us.sentry.io/4511103855034368",
		Release:          "willow@" + version,
		Environment:      env,
		EnableTracing:    true,
		TracesSampleRate: 1.0,
		EnableLogs:       true,
		AttachStacktrace: true,
	})
	if err != nil {
		return noop
	}

	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetTag("os", runtime.GOOS)
		scope.SetTag("arch", runtime.GOARCH)
	})

	enabled = true
	return func() {
		sentry.Flush(2 * time.Second)
	}
}

// Enabled returns whether telemetry is active.
func Enabled() bool {
	return enabled
}

// StartCommand begins a Sentry transaction for a CLI command.
// Returns a context carrying the transaction and a finish function.
// The finish function captures errors, emits metrics, and ends the transaction.
func StartCommand(ctx context.Context, command string) (context.Context, func(error)) {
	if !enabled {
		return ctx, func(error) {}
	}

	start := time.Now()

	tx := sentry.StartTransaction(ctx, "cli."+command,
		sentry.WithOpName("cli.command"),
		sentry.WithTransactionSource(sentry.SourceCustom),
	)
	tx.SetTag("command", command)

	return tx.Context(), func(err error) {
		if err != nil {
			tx.Status = sentry.SpanStatusInternalError
			sentry.CaptureException(err)
		} else {
			tx.Status = sentry.SpanStatusOK
		}

		logger := sentry.NewLogger(tx.Context())
		if err != nil {
			logger.Warn().
				String("command", command).
				Emitf("command failed: %s", err)
		} else {
			logger.Info().
				String("command", command).
				Emit("command completed")
		}

		elapsed := float64(time.Since(start).Milliseconds())
		meter := sentry.NewMeter(tx.Context())
		meter.Count("cli.command.count", 1,
			sentry.WithAttributes(attribute.String("command", command)),
		)
		meter.Distribution("cli.command.duration_ms", elapsed,
			sentry.WithAttributes(attribute.String("command", command)),
		)

		tx.Finish()
	}
}

// ResolveCommandName extracts the subcommand name from os.Args.
// Returns the first non-flag argument after the binary name, or "root" if none.
func ResolveCommandName(args []string) string {
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg
	}
	return "root"
}

func isOptedOut() bool {
	if v := os.Getenv("WILLOW_TELEMETRY"); v != "" {
		v = strings.ToLower(v)
		if v == "off" || v == "false" || v == "0" {
			return true
		}
	}

	cfg := config.Load("")
	if cfg.Telemetry != nil && !*cfg.Telemetry {
		return true
	}

	return false
}
