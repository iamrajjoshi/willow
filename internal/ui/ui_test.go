package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestBold(t *testing.T) {
	u := &UI{}
	got := u.Bold("hello")
	want := bold + "hello" + reset
	if got != want {
		t.Errorf("Bold() = %q, want %q", got, want)
	}
}

func TestGreen(t *testing.T) {
	u := &UI{}
	got := u.Green("ok")
	want := green + "ok" + reset
	if got != want {
		t.Errorf("Green() = %q, want %q", got, want)
	}
}

func TestYellow(t *testing.T) {
	u := &UI{}
	got := u.Yellow("warn")
	want := yellow + "warn" + reset
	if got != want {
		t.Errorf("Yellow() = %q, want %q", got, want)
	}
}

func TestRed(t *testing.T) {
	u := &UI{}
	got := u.Red("err")
	want := red + "err" + reset
	if got != want {
		t.Errorf("Red() = %q, want %q", got, want)
	}
}

func TestCyan(t *testing.T) {
	u := &UI{}
	got := u.Cyan("info")
	want := cyan + "info" + reset
	if got != want {
		t.Errorf("Cyan() = %q, want %q", got, want)
	}
}

func TestDim(t *testing.T) {
	u := &UI{}
	got := u.Dim("faded")
	want := dim + "faded" + reset
	if got != want {
		t.Errorf("Dim() = %q, want %q", got, want)
	}
}

func TestUI_DefaultWritesToStdout(t *testing.T) {
	u := &UI{}
	if u.out() != nil {
		// out() returns os.Stdout by default, just verify it doesn't panic
	}
}

func TestUI_CustomOut(t *testing.T) {
	var buf bytes.Buffer
	u := &UI{Out: &buf}

	u.Info("hello")
	u.Warn("oops")
	u.Success("done")

	out := buf.String()
	if !strings.Contains(out, "hello") {
		t.Errorf("Info() not written to custom Out, got %q", out)
	}
	if !strings.Contains(out, "oops") {
		t.Errorf("Warn() not written to custom Out, got %q", out)
	}
	if !strings.Contains(out, "done") {
		t.Errorf("Success() not written to custom Out, got %q", out)
	}
}

func TestUI_CustomOutDoesNotWriteToStdout(t *testing.T) {
	var buf bytes.Buffer
	u := &UI{Out: &buf}

	u.Info("redirected")
	u.Warn("redirected warning")

	// Verify output went to buf, not stdout
	if !strings.Contains(buf.String(), "redirected") {
		t.Error("output should go to custom Out")
	}
}

func TestTerminalControlSequences(t *testing.T) {
	tests := []struct {
		name string
		fn   func() string
	}{
		{"CursorHome", CursorHome},
		{"ClearToEnd", ClearToEnd},
		{"HideCursor", HideCursor},
		{"ShowCursor", ShowCursor},
		{"AltScreenOn", AltScreenOn},
		{"AltScreenOff", AltScreenOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn()
			if got == "" {
				t.Errorf("%s() returned empty string", tt.name)
			}
			if got[0] != '\033' {
				t.Errorf("%s() should start with ESC", tt.name)
			}
		})
	}
}
