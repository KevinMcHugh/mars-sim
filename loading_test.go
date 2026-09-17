package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// liveLoader builds a loader that draws its line art into buf, standing in for
// a terminal. newLoader cannot be used here: it decides on live by asking the
// real file descriptor.
func liveLoader(buf *bytes.Buffer) *loader { return &loader{out: buf, live: true} }

func TestLoaderPaintsBannerThenOneDotPerStep(t *testing.T) {
	var buf bytes.Buffer
	l := liveLoader(&buf)

	l.start()
	l.step()
	l.step()

	got := buf.String()
	// Each paint redraws the whole line from column 1, so the last one is what
	// the player is left looking at.
	want := "\r" + loadingBanner + "..\x1b[K"
	if !strings.HasSuffix(got, want) {
		t.Errorf("after two steps the line is %q, want it to end with %q", got, want)
	}
	if n := strings.Count(got, loadingBanner); n != 3 {
		t.Errorf("painted the banner %d times, want 3 (start + two steps)", n)
	}
}

func TestLoaderClearErasesTheLine(t *testing.T) {
	var buf bytes.Buffer
	l := liveLoader(&buf)

	l.start()
	buf.Reset()
	l.clear()

	if got, want := buf.String(), "\r\x1b[K"; got != want {
		t.Errorf("clear wrote %q, want %q", got, want)
	}
}

func TestLoaderNoteDoesNotRunIntoTheLoadingLine(t *testing.T) {
	var buf bytes.Buffer
	l := liveLoader(&buf)

	l.start()
	l.note("mars-sim: %s\n", "no cursor report")

	got := buf.String()
	// The message must start at column 1 on a line of its own, never appended
	// to the banner — that is the whole reason note exists.
	if strings.Contains(got, loadingBanner+"mars-sim:") {
		t.Errorf("note ran into the loading line: %q", got)
	}
	if !strings.Contains(got, "\r\x1b[Kmars-sim: no cursor report\n") {
		t.Errorf("note did not clear the line before printing: %q", got)
	}
	// ...and the loading line comes back after it.
	if !strings.HasSuffix(got, "\r"+loadingBanner+"\x1b[K") {
		t.Errorf("note did not repaint the loading line: %q", got)
	}
}

func TestLoaderNotLiveSkipsEscapesButKeepsNotes(t *testing.T) {
	var buf bytes.Buffer
	l := &loader{out: &buf, live: false}

	l.start()
	l.step()
	l.note("mars-sim: %s\n", "redirected")
	l.clear()

	got := buf.String()
	if got != "mars-sim: redirected\n" {
		t.Errorf("a redirected loader wrote %q, want only the note", got)
	}
	if strings.ContainsAny(got, "\r\x1b") {
		t.Errorf("a redirected loader emitted terminal control codes: %q", got)
	}
}

func TestNewLoaderOnANonTerminalIsNotLive(t *testing.T) {
	// A pipe is the shape a redirect takes, and the case that must not paint.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	if l := newLoader(w); l.live {
		t.Error("newLoader called a pipe live; it would write escapes into a redirect")
	}
}
