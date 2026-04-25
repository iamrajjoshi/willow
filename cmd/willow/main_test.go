package main

import "testing"

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
