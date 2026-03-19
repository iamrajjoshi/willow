package ui

import "testing"

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
