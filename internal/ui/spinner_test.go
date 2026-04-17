package ui

import (
	"errors"
	"os"
	"testing"
)

func TestSpinnerFrame_Cycles(t *testing.T) {
	u := &UI{}
	n := len(SpinnerFrames)
	for i := 0; i < n*3; i++ {
		if got, want := u.SpinnerFrame(i), SpinnerFrames[i%n]; got != want {
			t.Errorf("SpinnerFrame(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestSpinnerFrame_NegativeTick(t *testing.T) {
	u := &UI{}
	// Should not panic and should return a valid frame.
	got := u.SpinnerFrame(-5)
	if got == "" {
		t.Error("SpinnerFrame(-5) returned empty")
	}
}

func TestSpin_NonTTY_RunsFnAndReturnsErr(t *testing.T) {
	// stderr in `go test` is not a TTY, so Spin takes the no-animation path.
	u := &UI{}
	want := errors.New("boom")
	ran := false
	got := u.Spin("doing thing", func() error {
		ran = true
		return want
	})
	if !ran {
		t.Fatal("fn was not called")
	}
	if got != want {
		t.Errorf("Spin returned %v, want %v", got, want)
	}
}

func TestSpin_NoColor_SkipsAnimation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	u := &UI{}
	ran := false
	if err := u.Spin("x", func() error { ran = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Error("fn not called")
	}
	// Sanity: NO_COLOR was actually set (defensive).
	if os.Getenv("NO_COLOR") == "" {
		t.Fatal("NO_COLOR env not set by t.Setenv")
	}
}
