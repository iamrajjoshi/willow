package main

import (
	"os"
	"testing"
)

func discardOutput(t *testing.T, fn func()) {
	t.Helper()

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	defer devNull.Close()

	origStdout := os.Stdout
	origStderr := os.Stderr
	os.Stdout = devNull
	os.Stderr = devNull
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
	}()

	fn()
}

func TestTraceStderrRequested(t *testing.T) {
	tests := []struct {
		name string
		env  string
		args []string
		want bool
	}{
		{name: "env one", env: "1", args: []string{"willow"}, want: true},
		{name: "env true", env: "true", args: []string{"willow"}, want: true},
		{name: "env on", env: "on", args: []string{"willow"}, want: true},
		{name: "trace flag", args: []string{"willow", "--trace", "ls"}, want: true},
		{name: "single dash trace flag", args: []string{"willow", "-trace", "ls"}, want: true},
		{name: "disabled", env: "0", args: []string{"willow", "ls"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("WILLOW_TRACE", tt.env)
			if got := traceStderrRequested(tt.args); got != tt.want {
				t.Fatalf("traceStderrRequested(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestRunReturnsZeroForHelp(t *testing.T) {
	origArgs := os.Args
	os.Args = []string{"willow", "--help"}
	t.Cleanup(func() { os.Args = origArgs })

	discardOutput(t, func() {
		if got := run(); got != 0 {
			t.Fatalf("run(--help) = %d, want 0", got)
		}
	})
}

func TestRunReturnsOneForCLIError(t *testing.T) {
	origArgs := os.Args
	os.Args = []string{"willow", "dispatch"}
	t.Cleanup(func() { os.Args = origArgs })

	discardOutput(t, func() {
		if got := run(); got != 1 {
			t.Fatalf("run(dispatch without prompt) = %d, want 1", got)
		}
	})
}
