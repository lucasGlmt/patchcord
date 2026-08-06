package cli

import (
	"bytes"
	"testing"
)

func TestSpinner_NonTTY_Silent(t *testing.T) {
	var buf bytes.Buffer
	sp := newSpinner(&buf)
	sp.start("working…")
	sp.stop()

	if buf.Len() != 0 {
		t.Errorf("spinner should produce no output on a non-TTY writer, got %d bytes: %q", buf.Len(), buf.String())
	}
}

func TestSpinner_StopWithoutStart(t *testing.T) {
	var buf bytes.Buffer
	sp := newSpinner(&buf)
	// Must not panic.
	sp.stop()
	sp.stop()
}

func TestSpinner_DoubleStart(t *testing.T) {
	var buf bytes.Buffer
	sp := newSpinner(&buf)
	// Non-TTY: both calls are no-ops, must not panic.
	sp.start("a")
	sp.start("b")
	sp.stop()
}
