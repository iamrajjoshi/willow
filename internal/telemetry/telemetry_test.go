package telemetry

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/getsentry/sentry-go"
)

func TestIsOptedOut_EnvVar(t *testing.T) {
	tests := []struct {
		value    string
		optedOut bool
	}{
		{"off", true},
		{"OFF", true},
		{"false", true},
		{"False", true},
		{"0", true},
		{"on", false},
		{"true", false},
		{"1", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("WILLOW_TELEMETRY=%q", tt.value), func(t *testing.T) {
			os.Setenv("WILLOW_TELEMETRY", tt.value)
			defer os.Unsetenv("WILLOW_TELEMETRY")

			got := isOptedOut()
			if got != tt.optedOut {
				t.Errorf("isOptedOut() = %v, want %v", got, tt.optedOut)
			}
		})
	}
}

func TestInit_DisabledByEnvVar(t *testing.T) {
	enabled = false
	os.Setenv("WILLOW_TELEMETRY", "off")
	defer os.Unsetenv("WILLOW_TELEMETRY")

	cleanup := Init("dev")
	defer cleanup()

	if enabled {
		t.Error("expected telemetry to be disabled when WILLOW_TELEMETRY=off")
	}
}

func TestInit_EnabledByDefault(t *testing.T) {
	enabled = false
	os.Unsetenv("WILLOW_TELEMETRY")

	cleanup := Init("dev")
	defer cleanup()

	if !enabled {
		t.Error("expected telemetry to be enabled by default")
	}

	// Reset for other tests
	enabled = false
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
