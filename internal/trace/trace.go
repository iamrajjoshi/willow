// Package trace emits timing data to stderr when enabled via --trace or
// WILLOW_TRACE. Calls are otherwise no-op.
//
//	done := trace.Span(ctx, "git.fetch")
//	defer done()
package trace

import (
	"context"
	"fmt"
	"os"
	"time"
)

type Tracer struct {
	stderr bool
	start  time.Time
}

type ctxKey struct{}

func WithTracer(ctx context.Context, t *Tracer) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

// FromContext returns the Tracer bound to ctx, or a disabled tracer when
// none is set. Never returns nil.
func FromContext(ctx context.Context) *Tracer {
	if t, ok := ctx.Value(ctxKey{}).(*Tracer); ok && t != nil {
		return t
	}
	return &Tracer{}
}

// Span times a block of work. The returned func ends the span.
func Span(ctx context.Context, label string) func() {
	return FromContext(ctx).StartCtx(ctx, label)
}

// New returns a Tracer. When stderr is true, spans print to stderr as
// they finish.
func New(stderr bool) *Tracer {
	return &Tracer{
		stderr: stderr,
		start:  time.Now(),
	}
}

var noop = func() {}

func (t *Tracer) StartCtx(ctx context.Context, label string) func() {
	stderr := t != nil && t.stderr
	if !stderr {
		return noop
	}

	start := time.Now()
	return func() {
		if stderr {
			fmt.Fprintf(os.Stderr, "[trace] %-30s %s\n", label, formatDuration(time.Since(start)))
		}
	}
}

// Start is the ctx-less form.
func (t *Tracer) Start(label string) func() {
	return t.StartCtx(context.Background(), label)
}

// Total prints elapsed time since New. Stderr only.
func (t *Tracer) Total() {
	if t == nil || !t.stderr {
		return
	}
	d := time.Since(t.start)
	fmt.Fprintf(os.Stderr, "[trace] %-30s %s\n", "TOTAL", formatDuration(d))
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%.0fµs", float64(d.Microseconds()))
	case d < time.Second:
		return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000)
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}
