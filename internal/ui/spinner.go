package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// SpinnerFrames is a 10-frame braille spinner cycle.
var SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SpinnerFrame returns the glyph for the given tick count.
func (u *UI) SpinnerFrame(tick int) string {
	if tick < 0 {
		tick = -tick
	}
	return SpinnerFrames[tick%len(SpinnerFrames)]
}

// Spin runs fn while animating a braille spinner + label on stderr. It
// returns fn's error and always clears the spinner line before returning.
//
// When stderr is not a TTY or NO_COLOR is set, Spin prints the label once
// and runs fn without animation so log output stays clean.
func (u *UI) Spin(label string, fn func() error) error {
	if !stderrIsTTY() || os.Getenv("NO_COLOR") != "" {
		fmt.Fprintln(os.Stderr, label)
		return fn()
	}

	s := newSpinner(os.Stderr, label)
	s.start()
	err := fn()
	s.stop()
	return err
}

type spinner struct {
	w      io.Writer
	label  string
	mu     sync.Mutex
	stopCh chan struct{}
	done   chan struct{}
}

func newSpinner(w io.Writer, label string) *spinner {
	return &spinner{
		w:      w,
		label:  label,
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
}

func (s *spinner) start() {
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		frame := 0
		u := &UI{}
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.mu.Lock()
				fmt.Fprintf(s.w, "\r%s %s", u.Cyan(SpinnerFrames[frame%len(SpinnerFrames)]), s.label)
				s.mu.Unlock()
				frame++
			}
		}
	}()
}

func (s *spinner) stop() {
	close(s.stopCh)
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprint(s.w, "\r\033[K")
}

func stderrIsTTY() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}
