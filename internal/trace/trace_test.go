package trace

import (
	"testing"
	"time"
)

func TestNew_Disabled(t *testing.T) {
	tr := New(false)
	if tr.enabled {
		t.Error("tracer should be disabled")
	}
}

func TestNew_Enabled(t *testing.T) {
	tr := New(true)
	if !tr.enabled {
		t.Error("tracer should be enabled")
	}
}

func TestStart_DisabledReturnsNoop(t *testing.T) {
	tr := New(false)
	done := tr.Start("test step")
	done() // should not panic
	if len(tr.steps) != 0 {
		t.Errorf("expected 0 steps when disabled, got %d", len(tr.steps))
	}
}

func TestStart_EnabledRecordsStep(t *testing.T) {
	tr := New(true)
	done := tr.Start("test step")
	time.Sleep(1 * time.Millisecond)
	done()
	if len(tr.steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(tr.steps))
	}
	if tr.steps[0].label != "test step" {
		t.Errorf("label = %q, want %q", tr.steps[0].label, "test step")
	}
	if tr.steps[0].duration == 0 {
		t.Error("duration should be > 0")
	}
}

func TestTotal_DisabledNoOp(t *testing.T) {
	tr := New(false)
	tr.Total() // should not panic
}

func TestFormatDuration_Microseconds(t *testing.T) {
	got := formatDuration(500 * time.Microsecond)
	if got != "500µs" {
		t.Errorf("formatDuration(500µs) = %q, want %q", got, "500µs")
	}
}

func TestFormatDuration_Milliseconds(t *testing.T) {
	got := formatDuration(15 * time.Millisecond)
	if got != "15.0ms" {
		t.Errorf("formatDuration(15ms) = %q, want %q", got, "15.0ms")
	}
}

func TestFormatDuration_Seconds(t *testing.T) {
	got := formatDuration(2500 * time.Millisecond)
	if got != "2.50s" {
		t.Errorf("formatDuration(2.5s) = %q, want %q", got, "2.50s")
	}
}
