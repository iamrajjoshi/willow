package telemetry

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/iamrajjoshi/willow/internal/config"
	willowerrors "github.com/iamrajjoshi/willow/internal/errors"
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
	pendingEvents.Store(false)
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
	pendingEvents.Store(false)
	os.Unsetenv("WILLOW_TELEMETRY")

	cleanup := Init("dev")
	defer cleanup()

	if enabled {
		t.Error("expected telemetry to be disabled by default")
	}
}

func TestStartCommand_NoopWhenDisabled(t *testing.T) {
	enabled = false
	pendingEvents.Store(false)

	ctx, finish := StartCommand(context.Background(), "test")
	if ctx == nil {
		t.Error("expected non-nil context")
	}

	finish(nil)
	finish(fmt.Errorf("test error"))
}

func TestStartCommand_IgnoresSuccessAndUserErrors(t *testing.T) {
	sentry.Init(sentry.ClientOptions{})
	enabled = true
	pendingEvents.Store(false)
	t.Cleanup(func() {
		enabled = false
		pendingEvents.Store(false)
	})

	_, finishSuccess := StartCommand(context.Background(), "ls")
	finishSuccess(nil)
	if pendingEvents.Load() {
		t.Fatal("successful command should not mark telemetry for flushing")
	}

	_, finishUser := StartCommand(context.Background(), "ls")
	finishUser(willowerrors.Userf("bad input"))
	if pendingEvents.Load() {
		t.Fatal("user error should not mark telemetry for flushing")
	}
}

func TestStartCommand_CapturesSystemErrors(t *testing.T) {
	sentry.Init(sentry.ClientOptions{})
	enabled = true
	pendingEvents.Store(false)
	t.Cleanup(func() {
		enabled = false
		pendingEvents.Store(false)
	})

	_, finish := StartCommand(context.Background(), "ls")
	finish(fmt.Errorf("boom"))
	if !pendingEvents.Load() {
		t.Fatal("system error should mark telemetry for flushing")
	}
}

func TestCaptureException_NoOpWhenDisabled(t *testing.T) {
	enabled = false
	pendingEvents.Store(false)

	CaptureException(fmt.Errorf("boom"))
	if pendingEvents.Load() {
		t.Fatal("disabled telemetry should not mark pending events")
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

func TestIsEnabled_Config(t *testing.T) {
	isolateHome(t)
	cfg := config.DefaultConfig()
	cfg.Telemetry = config.BoolPtr(true)
	if err := config.Save(cfg, config.GlobalConfigPath()); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	if !isEnabled() {
		t.Fatal("expected telemetry to be enabled from config")
	}
}
