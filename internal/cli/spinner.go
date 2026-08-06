package cli

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
)

// spinner displays a small animated indicator on a terminal while a
// long-running operation is in progress.  When the writer is not a
// terminal (piped output, tests using bytes.Buffer, etc.) the spinner
// is completely silent — no output at all.
type spinner struct {
	w    io.Writer
	mu   sync.Mutex
	done chan struct{}
	msg  string
	tty  bool
}

var spinnerFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// newSpinner creates a spinner that will write to w.  The spinner does
// not start automatically — call start.
func newSpinner(w io.Writer) *spinner {
	tty := false
	if f, ok := w.(*os.File); ok {
		tty = isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
	}
	return &spinner{w: w, tty: tty}
}

// start begins displaying msg with an animated spinner.  Calling start
// while the spinner is already running is a no-op.
func (s *spinner) start(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.tty || s.done != nil {
		return
	}

	s.msg = msg
	s.done = make(chan struct{})

	go func() {
		tick := time.NewTicker(80 * time.Millisecond)
		defer tick.Stop()

		i := 0
		for {
			select {
			case <-s.done:
				// Clear the spinner line before returning.
				fmt.Fprintf(s.w, "\r\033[K")
				return
			case <-tick.C:
				frame := spinnerFrames[i%len(spinnerFrames)]
				fmt.Fprintf(s.w, "\r%s %s", frame, s.msg)
				i++
			}
		}
	}()
}

// stop stops the spinner and clears its line.  Safe to call multiple
// times or when the spinner was never started.
func (s *spinner) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.done == nil {
		return
	}

	close(s.done)
	// Give the goroutine a moment to clear the line.
	time.Sleep(10 * time.Millisecond)
	s.done = nil
}
