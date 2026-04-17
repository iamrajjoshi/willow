// Package trace emits timing data to stderr (dev, via --trace/WILLOW_TRACE)
// and/or to a registered SpanHook (prod, wired to Sentry by the telemetry
// package). Both sinks are optional; calls are no-op when neither is active.
//
//	done := trace.Span(ctx, "git.fetch")
//	defer done()
package trace

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

type Tracer struct {
	stderr bool
	start  time.Time
}

// SpanHook is invoked on every traced operation. The returned func runs
// when the span ends.
type SpanHook func(ctx context.Context, label string) func()

var hook atomic.Pointer[SpanHook]

// SetSpanHook installs the global span hook. Pass nil to clear.
func SetSpanHook(h SpanHook) {
	if h == nil {
		hook.Store(nil)
		return
	}
	hook.Store(&h)
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
	var h SpanHook
	if p := hook.Load(); p != nil {
		h = *p
	}
	stderr := t != nil && t.stderr
	if !stderr && h == nil {
		return noop
	}

	start := time.Now()
	var hookFinish func()
	if h != nil {
		hookFinish = h(ctx, label)
	}
	return func() {
		if hookFinish != nil {
			hookFinish()
		}
		if stderr {
			fmt.Fprintf(os.Stderr, "[trace] %-30s %s\n", label, formatDuration(time.Since(start)))
		}
	}
}

// Start is the ctx-less form; prefer Span(ctx, ...) when ctx is available
// so Sentry spans can link to a parent transaction.
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
