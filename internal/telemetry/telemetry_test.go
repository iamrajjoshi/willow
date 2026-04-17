package telemetry

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/iamrajjoshi/willow/internal/trace"
)

// isolateHome points HOME at a fresh temp dir so config.Load("") can't pick up
// the developer's real ~/.config/willow/config.json during tests.
func isolateHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestIsEnabled_EnvVar(t *testing.T) {
	tests := []struct {
		value   string
		enabled bool
	}{
		{"on", true},
		{"ON", true},
		{"true", true},
		{"True", true},
		{"1", true},
		{"off", false},
		{"false", false},
		{"0", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("WILLOW_TELEMETRY=%q", tt.value), func(t *testing.T) {
			isolateHome(t)
			os.Setenv("WILLOW_TELEMETRY", tt.value)
			defer os.Unsetenv("WILLOW_TELEMETRY")

			got := isEnabled()
			if got != tt.enabled {
				t.Errorf("isEnabled() = %v, want %v", got, tt.enabled)
			}
		})
	}
}

func TestInit_DisabledByEnvVar(t *testing.T) {
	isolateHome(t)
	enabled = false
	os.Setenv("WILLOW_TELEMETRY", "off")
	defer os.Unsetenv("WILLOW_TELEMETRY")

	cleanup := Init("dev")
	defer cleanup()

	if enabled {
		t.Error("expected telemetry to be disabled when WILLOW_TELEMETRY=off")
	}
}

func TestInit_DisabledByDefault(t *testing.T) {
	isolateHome(t)
	enabled = false
	os.Unsetenv("WILLOW_TELEMETRY")

	cleanup := Init("dev")
	defer cleanup()

	if enabled {
		t.Error("expected telemetry to be disabled by default")
	}
}

func TestStartCommand_NoopWhenDisabled(t *testing.T) {
	enabled = false

	ctx, finish := StartCommand(context.Background(), "test")
	if ctx == nil {
		t.Error("expected non-nil context")
	}

	// Should not panic
	finish(nil)
	finish(fmt.Errorf("test error"))
}

func TestStartCommand_WithTransaction(t *testing.T) {
	// Use empty DSN so events are silently discarded, not sent to real Sentry
	sentry.Init(sentry.ClientOptions{EnableTracing: true, EnableLogs: true})
	enabled = true
	defer func() { enabled = false }()

	ctx, finish := StartCommand(context.Background(), "new")
	if ctx == nil {
		t.Error("expected non-nil context")
	}

	// Should not panic with nil error
	finish(nil)
}

func TestStartCommand_WithError(t *testing.T) {
	// Use empty DSN so events are silently discarded, not sent to real Sentry
	sentry.Init(sentry.ClientOptions{EnableTracing: true, EnableLogs: true})
	enabled = true
	defer func() { enabled = false }()

	_, finish := StartCommand(context.Background(), "clone")

	// Should not panic with real error
	finish(fmt.Errorf("test error: failed to clone"))
}

func TestInit_DoesNotInstallSpanHookWhenDisabled(t *testing.T) {
	isolateHome(t)
	trace.SetSpanHook(nil)
	t.Cleanup(func() { trace.SetSpanHook(nil) })

	enabled = false
	os.Unsetenv("WILLOW_TELEMETRY")

	cleanup := Init("dev")
	defer cleanup()

	// A probe hook registered after Init must be the only one firing;
	// if Init had installed its own, we'd see two invocations.
	count := 0
	trace.SetSpanHook(func(ctx context.Context, label string) func() {
		count++
		return func() {}
	})
	trace.Span(context.Background(), "probe")()
	if count != 1 {
		t.Errorf("span hook invoked %d times, want 1 (Init should not have installed one)", count)
	}
}

func TestResolveCommandName(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"simple command", []string{"willow", "new", "branch"}, "new"},
		{"flags before command", []string{"willow", "--verbose", "clone", "url"}, "clone"},
		{"short flag with value", []string{"willow", "-C", "/tmp", "ls"}, "/tmp"},
		{"no subcommand", []string{"willow"}, "root"},
		{"only flags", []string{"willow", "--version"}, "root"},
		{"flag with equals", []string{"willow", "--trace", "sync"}, "sync"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveCommandName(tt.args)
			if got != tt.want {
				t.Errorf("ResolveCommandName(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
