package telemetry

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/getsentry/sentry-go/attribute"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errs"
)

var enabled bool

// Init initializes Sentry if telemetry is enabled.
// Returns a cleanup function that flushes buffered events.
func Init(version string) func() {
	noop := func() {}

	if !isEnabled() {
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
		scope.SetUser(sentry.User{ID: machineID()})
	})

	enabled = true
	return func() {
		sentry.Flush(2 * time.Second)
	}
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
			if errs.IsUser(err) {
				tx.Status = sentry.SpanStatusInvalidArgument
			} else {
				tx.Status = sentry.SpanStatusInternalError
				sentry.CaptureException(err)
			}
		} else {
			tx.Status = sentry.SpanStatusOK
		}

		elapsed := float64(time.Since(start).Milliseconds())

		logger := sentry.NewLogger(tx.Context())
		if err != nil {
			errorType := "system"
			if errs.IsUser(err) {
				errorType = "user"
			}
			logger.Warn().
				String("command", command).
				Float64("duration_ms", elapsed).
				String("status", "error").
				String("error_type", errorType).
				Emitf("command failed: %s", err)
		} else {
			logger.Info().
				String("command", command).
				Float64("duration_ms", elapsed).
				String("status", "ok").
				Emit("command completed")
		}
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

func machineID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	hash := sha256.Sum256([]byte(hostname))
	return fmt.Sprintf("%x", hash[:8])
}

func isEnabled() bool {
	if v := os.Getenv("WILLOW_TELEMETRY"); v != "" {
		v = strings.ToLower(v)
		return v == "on" || v == "true" || v == "1"
	}

	cfg := config.Load("")
	if cfg.Telemetry != nil && *cfg.Telemetry {
		return true
	}

	return false
}
