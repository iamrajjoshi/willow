package telemetry

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/errors"
)

var enabled bool
var pendingEvents atomic.Bool

// Init initializes Sentry if telemetry is enabled.
// Returns a cleanup function that flushes buffered exceptions before exit.
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
	pendingEvents.Store(false)
	return func() {
		enabled = false
		if pendingEvents.Load() {
			sentry.Flush(2 * time.Second)
		}
		pendingEvents.Store(false)
	}
}

// StartCommand tracks command failures for telemetry.
// Successful commands do not emit telemetry.
func StartCommand(ctx context.Context, command string) (context.Context, func(error)) {
	if !enabled {
		return ctx, func(error) {}
	}

	start := time.Now()
	return ctx, func(err error) {
		if err == nil || errors.IsUser(err) {
			return
		}

		elapsed := float64(time.Since(start).Milliseconds())
		captureExceptionWithScope(err, func(scope *sentry.Scope) {
			scope.SetTag("command", command)
			scope.SetExtra("duration_ms", elapsed)
			scope.SetExtra("status", "system_error")
		})
	}
}

// CaptureException reports an error to Sentry and ensures the current process
// flushes before exit.
func CaptureException(err error) {
	captureExceptionWithScope(err, nil)
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

func captureExceptionWithScope(err error, configure func(*sentry.Scope)) {
	if err == nil || !enabled {
		return
	}

	pendingEvents.Store(true)
	sentry.WithScope(func(scope *sentry.Scope) {
		if configure != nil {
			configure(scope)
		}
		sentry.CaptureException(err)
	})
}
