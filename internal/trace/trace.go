// Package trace provides lightweight performance timing that targets two
// independent channels:
//
//   - stderr, when the user passes --trace or sets WILLOW_TRACE (dev use)
//   - an external hook, typically Sentry child spans (prod telemetry)
//
// Both are optional. When neither is enabled, every tracing call compiles
// down to a single branch and a returned no-op closure.
//
// Use the context-based API for new code:
//
//	done := trace.Span(ctx, "git.fetch")
//	defer done()
//
// Tracer-as-parameter is kept for the original call sites in internal/cli
// commands that predate the context-based API.
package trace

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

// Tracer controls stderr output for a single command invocation.
type Tracer struct {
	stderr bool
	start  time.Time
}

// SpanHook is called on every traced operation when registered. The
// returned func is invoked when the span ends. Typically wired to
// sentry.StartSpan by the telemetry package.
type SpanHook func(ctx context.Context, label string) func()

// hook is a global so trace.Span works without threading a bridge through
// every call site. telemetry.Init installs it; tests may overwrite.
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

// WithTracer attaches t to ctx so Span/FromContext can find it.
func WithTracer(ctx context.Context, t *Tracer) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

// FromContext returns the Tracer bound to ctx, or a disabled no-op tracer
// when none is set. Never returns nil.
func FromContext(ctx context.Context) *Tracer {
	if t, ok := ctx.Value(ctxKey{}).(*Tracer); ok && t != nil {
		return t
	}
	return &Tracer{}
}

// Span is the primary entrypoint for instrumenting code. It consults the
// tracer on ctx and the global span hook; the returned func ends the span.
//
//	done := trace.Span(ctx, "MergedBranchSet")
//	defer done()
func Span(ctx context.Context, label string) func() {
	return FromContext(ctx).StartCtx(ctx, label)
}

// New returns a Tracer. When stderr is true, Start/StartCtx print timings
// to stderr as they finish.
func New(stderr bool) *Tracer {
	return &Tracer{
		stderr: stderr,
		start:  time.Now(),
	}
}

var noop = func() {}

// StartCtx starts a span, emitting to stderr (when the tracer is in
// stderr mode) and/or to the registered span hook (when telemetry is on).
// Returns a no-op closure if neither sink is listening.
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

// Start is the legacy entrypoint for call sites without a ctx. Sentry
// spans created this way won't link to a parent transaction — prefer
// Span(ctx, ...) when a ctx is available.
func (t *Tracer) Start(label string) func() {
	return t.StartCtx(context.Background(), label)
}

// Total prints the elapsed time since the Tracer was created. Stderr only;
// the telemetry package records overall command duration separately.
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
