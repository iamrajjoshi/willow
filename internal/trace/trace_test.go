package trace

import (
	"context"
	"testing"
	"time"
)

func TestNew_Disabled(t *testing.T) {
	tr := New(false)
	if tr.stderr {
		t.Error("tracer should be disabled")
	}
}

func TestNew_Enabled(t *testing.T) {
	tr := New(true)
	if !tr.stderr {
		t.Error("tracer should be enabled")
	}
}

func TestStart_DisabledReturnsNoop(t *testing.T) {
	tr := New(false)
	done := tr.Start("test step")
	done()
}

func TestTotal_DisabledNoOp(t *testing.T) {
	tr := New(false)
	tr.Total()
}

func TestFromContext_MissingReturnsNoopTracer(t *testing.T) {
	tr := FromContext(context.Background())
	if tr == nil {
		t.Fatal("FromContext returned nil")
	}
	if tr.stderr {
		t.Error("tracer from empty ctx should not be in stderr mode")
	}
}

func TestFromContext_RoundTrip(t *testing.T) {
	tr := New(true)
	ctx := WithTracer(context.Background(), tr)
	if got := FromContext(ctx); got != tr {
		t.Errorf("FromContext returned different tracer, got %p want %p", got, tr)
	}
}

func TestSpan_NoStderrIsNoop(t *testing.T) {
	done := Span(context.Background(), "label")
	done() // must not panic
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
