package trace

import (
	"fmt"
	"os"
	"time"
)

type Tracer struct {
	enabled bool
	start   time.Time
	steps   []step
}

type step struct {
	label    string
	duration time.Duration
}

func New(enabled bool) *Tracer {
	return &Tracer{
		enabled: enabled,
		start:   time.Now(),
	}
}

// Step records a completed step and prints its duration.
func (t *Tracer) Step(label string, start time.Time) {
	if !t.enabled {
		return
	}
	d := time.Since(start)
	t.steps = append(t.steps, step{label: label, duration: d})
	fmt.Fprintf(os.Stderr, "[trace] %-25s %s\n", label, formatDuration(d))
}

// Total prints the total elapsed time since the tracer was created.
func (t *Tracer) Total() {
	if !t.enabled {
		return
	}
	d := time.Since(t.start)
	fmt.Fprintf(os.Stderr, "[trace] %-25s %s\n", "TOTAL", formatDuration(d))
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
