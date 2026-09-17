package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
)

// Startup is short but not instant, and for most of it the screen is blank: the
// terminal has just spent however long `go run .` needed to compile, and then
// the process itself measures glyph widths and generates a world before Bubble
// Tea paints anything. The loader fills that gap with a line that says the game
// is alive and moving.
//
// It is deliberately dumb — no spinner goroutine, no timers. Each startup phase
// calls step() when it finishes, so a dot is evidence that a phase completed
// rather than an animation that would keep twirling if the process wedged.

// loadingBanner leads the loading line; a dot follows per completed phase.
const loadingBanner = "Mars awaits"

// loader draws the single-line startup indicator. The zero value is unusable;
// build one with newLoader.
type loader struct {
	out  io.Writer
	live bool // false when out is not a terminal: print notes, skip the line art
	dots int
}

// newLoader returns a loader that draws on out, which is expected to be
// os.Stderr.
//
// Two reasons for stderr rather than stdout. The alt screen is about to cover
// stdout, and -headless prints machine-readable stats there that a redirect
// should not find studded with dots. And a loading line is progress, not
// output: redirecting stdout to a file should not capture it.
//
// When out is not a terminal the line art is skipped entirely — carriage
// returns and erase-line escapes are noise in a log file — but note() still
// prints, so a redirected run loses nothing but the animation.
func newLoader(out *os.File) *loader {
	return &loader{out: out, live: out != nil && term.IsTerminal(out.Fd())}
}

// start paints the banner with no dots yet.
func (l *loader) start() { l.paint() }

// step marks one startup phase complete and adds a dot.
func (l *loader) step() {
	l.dots++
	l.paint()
}

// note prints a startup message (which must end in a newline) above the loading
// line, then redraws the line. Writing to stderr directly during startup would
// otherwise append the message to the half-finished loading line.
func (l *loader) note(format string, args ...any) {
	l.clear()
	fmt.Fprintf(l.out, format, args...)
	l.paint()
}

// clear erases the loading line and leaves the cursor at its start, so whatever
// comes next — the TUI's alt screen, the headless banner — starts clean.
func (l *loader) clear() {
	if !l.live {
		return
	}
	fmt.Fprint(l.out, "\r\x1b[K")
}

// paint redraws the whole line rather than appending a single dot, because the
// line is not exclusively ours: the glyph probe parks at column 1 and erases
// the line for every glyph it measures (see internal/ui/tui/probe.go). Redrawing
// from column 1 means the probe cannot leave us with a mangled line — the next
// step() repaints it whole.
func (l *loader) paint() {
	if !l.live {
		return
	}
	fmt.Fprintf(l.out, "\r%s%s\x1b[K", loadingBanner, strings.Repeat(".", l.dots))
}
