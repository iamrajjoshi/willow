package trace

import (
	"context"
	"sync/atomic"
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

func TestSpan_NoHookNoStderrIsNoop(t *testing.T) {
	// Ensure no hook is registered.
	SetSpanHook(nil)
	t.Cleanup(func() { SetSpanHook(nil) })

	done := Span(context.Background(), "label")
	done() // must not panic
}

func TestSpan_InvokesRegisteredHook(t *testing.T) {
	var startCount, finishCount atomic.Int32
	var gotLabel string
	SetSpanHook(func(ctx context.Context, label string) func() {
		startCount.Add(1)
		gotLabel = label
		return func() { finishCount.Add(1) }
	})
	t.Cleanup(func() { SetSpanHook(nil) })

	done := Span(context.Background(), "hook-label")
	if startCount.Load() != 1 {
		t.Fatalf("hook not invoked on start")
	}
	if finishCount.Load() != 0 {
		t.Fatalf("finisher invoked too early")
	}
	done()
	if finishCount.Load() != 1 {
		t.Fatalf("finisher not invoked")
	}
	if gotLabel != "hook-label" {
		t.Errorf("hook got label %q, want %q", gotLabel, "hook-label")
	}
}

func TestSpan_HookRunsEvenWhenStderrOff(t *testing.T) {
	var ran atomic.Bool
	SetSpanHook(func(ctx context.Context, label string) func() {
		return func() { ran.Store(true) }
	})
	t.Cleanup(func() { SetSpanHook(nil) })

	// Tracer with stderr off — hook should still fire.
	ctx := WithTracer(context.Background(), New(false))
	Span(ctx, "silent")()
	if !ran.Load() {
		t.Error("hook should fire even when tracer is in silent mode")
	}
}

func TestSetSpanHook_Nil_ClearsHook(t *testing.T) {
	var ran atomic.Bool
	SetSpanHook(func(ctx context.Context, label string) func() {
		return func() { ran.Store(true) }
	})
	SetSpanHook(nil)

	Span(context.Background(), "cleared")()
	if ran.Load() {
		t.Error("cleared hook was invoked")
	}
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
