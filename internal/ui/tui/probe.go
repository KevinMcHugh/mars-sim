package tui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
)

// How wide a terminal paints an emoji is not derivable from the string. Width
// tables — ours and the terminal's — are snapshots of a Unicode version, and
// two programs built against different snapshots disagree. Every static fix for
// this bug class is a better guess; this file stops guessing and asks.
//
// The method is the cursor position report. Park the cursor at a known column,
// print a glyph, then send ESC[6n: the terminal replies ESC[<row>;<col>R with
// where the cursor ended up, and the distance it moved is the width that
// terminal just painted, by definition. It is the only authoritative answer
// available to a program that cannot see the screen.
//
// The probe runs before Bubble Tea takes the terminal, because it needs raw
// mode and the input stream to itself. Everything about it degrades to the
// static registry: not a TTY, no reply, a malformed reply, a timeout — any of
// those and we keep the widths we shipped with.

// probeTimeout bounds the wait for a cursor position report. A terminal that
// supports CPR answers in microseconds; one that does not will never answer, and
// this is how long the game is willing to pause at startup to find out.
const probeTimeout = 250 * time.Millisecond

// probeBatch is how many glyphs are measured per round trip.
//
// Composed colonist glyphs are combinatorial — six figures times five skin
// tones times four hair states — so there are around ninety glyphs to measure.
// One query-and-wait each is imperceptible on a local terminal and several
// seconds over a slow SSH link. Terminals process their input stream in order
// and queue a reply per query, so the whole batch can go out in one write and
// the replies come back in the same order.
//
// The batch is bounded because those replies sit in the terminal's input buffer
// until we read them: thirty-two replies is about three hundred bytes, far
// inside any plausible buffer.
const probeBatch = 32

// GlyphCheck is what the probe found.
type GlyphCheck struct {
	Downgraded bool   // the whole UI was switched to ASCII glyphs
	Reduced    int    // composed glyphs that dropped to a simpler rung
	Detail     string // one line describing the outcome, for logging
}

// UseASCIIGlyphs forces the ASCII glyph set, for a player whose terminal draws
// emoji badly in a way the probe cannot detect — a font with no glyph for a
// code point still advances the expected number of cells.
func UseASCIIGlyphs() { useASCIIGlyphs() }

// VerifyGlyphWidths measures every registered glyph against the terminal and
// switches the UI to ASCII glyphs if any is painted at a width the registry
// does not expect.
//
// It returns an error only when the probe could not run at all, which is not a
// failure — just a terminal we cannot interrogate. The static registry stands
// in that case.
//
// The probe opens /dev/tty rather than using os.Stdin, for two reasons. It must
// never compete with Bubble Tea for stdin: if a read here outlived the probe it
// could swallow a keypress meant for the game. And closing this descriptor on
// the way out is what unblocks a read that is still waiting on a terminal that
// never answered — there is no portable way to cancel a read on stdin, but
// there is always a way to close a file we opened ourselves.
func VerifyGlyphWidths() (GlyphCheck, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return GlyphCheck{}, fmt.Errorf("opening the terminal: %w", err)
	}
	// This close is load-bearing, not just tidiness: it releases any read still
	// blocked on a silent terminal.
	defer tty.Close()

	if !term.IsTerminal(tty.Fd()) {
		return GlyphCheck{}, errors.New("not a terminal")
	}

	state, err := term.MakeRaw(tty.Fd())
	if err != nil {
		return GlyphCheck{}, fmt.Errorf("raw mode: %w", err)
	}
	defer term.Restore(tty.Fd(), state)

	var out io.Writer = tty
	reader := bufio.NewReader(tty)

	// Probe the empty string first. This measures nothing, so a terminal that
	// speaks CPR must report the column we parked at — and one that does not
	// speak CPR, or is behind a multiplexer that swallows the query, fails here
	// rather than after we have drawn glyphs into the user's scrollback.
	if base, err := measureBatch(reader, out, []string{""}); err != nil {
		return GlyphCheck{}, fmt.Errorf("cursor position report unsupported: %w", err)
	} else if base[0] != 0 {
		return GlyphCheck{}, fmt.Errorf("cursor position report is unreliable: empty string measured %d cells", base[0])
	}

	// Sorted, so a mismatch report reads the same way twice.
	symbols := make([]string, 0, len(glyphRegistry))
	for symbol := range glyphRegistry {
		if symbol != glyphFloor { // plain spaces; nothing to disagree about
			symbols = append(symbols, symbol)
		}
	}
	sort.Strings(symbols)

	rejected := make(map[string]bool)
	var atomicMismatches, composedMismatches []string
	for start := 0; start < len(symbols); start += probeBatch {
		end := min(start+probeBatch, len(symbols))
		batch := symbols[start:end]

		painted, err := measureBatch(reader, out, batch)
		if err != nil {
			return GlyphCheck{}, fmt.Errorf("measuring glyphs %d-%d: %w", start, end, err)
		}
		for i, symbol := range batch {
			g := glyphRegistry[symbol]
			if painted[i] == g.cells {
				continue
			}
			rejected[symbol] = true
			note := fmt.Sprintf("%s painted %d cells, expected %d", symbol, painted[i], g.cells)
			if g.composed() {
				composedMismatches = append(composedMismatches, note)
			} else {
				atomicMismatches = append(atomicMismatches, note)
			}
		}
	}

	// An atomic glyph is a single code point every width table agrees on. A
	// terminal that gets one wrong is not one to trust with the rest, so that
	// is the case that downgrades everything.
	if len(atomicMismatches) > 0 {
		useASCIIGlyphs()
		return GlyphCheck{
			Downgraded: true,
			Detail: fmt.Sprintf("terminal disagrees about %d basic glyph width(s), using ASCII glyphs: %s",
				len(atomicMismatches), strings.Join(atomicMismatches, "; ")),
		}, nil
	}

	// A composed glyph failing means only that this terminal will not fuse that
	// sequence. Each one drops to the rung below it and the rest stay emoji.
	if len(composedMismatches) > 0 {
		reduceRejectedGlyphs(rejected)
		return GlyphCheck{
			Reduced: len(composedMismatches),
			Detail: fmt.Sprintf("terminal will not fuse %d of %d glyphs; those fall back to a simpler figure: %s",
				len(composedMismatches), len(symbols), strings.Join(composedMismatches, "; ")),
		}, nil
	}

	return GlyphCheck{
		Detail: fmt.Sprintf("terminal paints all %d glyphs at the expected width", len(symbols)),
	}, nil
}

// measureBatch writes each symbol at a known column and returns how many
// columns the cursor advanced for each — the width that terminal paints them
// at.
//
// Per symbol the sequence is \r to park the cursor in column 1, the symbol, and
// ESC[6n to ask where the cursor ended up. Those go out for the whole batch in
// one write: the terminal handles its input in order, so the replies come back
// in the same order, and a batch costs one round trip rather than one each.
// Every symbol is measured from column 1, so each reply stands alone and a
// symbol that paints unexpectedly wide cannot skew the next one.
//
// A trailing \r\x1b[K returns to column 1 and erases the line, so nothing the
// probe drew survives into the session.
func measureBatch(reader *bufio.Reader, out io.Writer, symbols []string) ([]int, error) {
	var query strings.Builder
	for _, s := range symbols {
		query.WriteString("\r")
		query.WriteString(s)
		query.WriteString("\x1b[6n")
	}
	if _, err := io.WriteString(out, query.String()); err != nil {
		return nil, err
	}

	widths := make([]int, len(symbols))
	for i := range symbols {
		col, err := readCursorColumn(reader)
		if err != nil {
			return nil, err
		}
		// Columns are 1-based, and every symbol started in column 1.
		widths[i] = col - 1
	}

	if _, err := io.WriteString(out, "\r\x1b[K"); err != nil {
		return nil, err
	}
	return widths, nil
}

// readCursorColumn reads one ESC[<row>;<col>R report, discarding anything
// before the escape (a terminal may have queued other input).
func readCursorColumn(reader *bufio.Reader) (int, error) {
	deadline := time.After(probeTimeout)
	type result struct {
		col int
		err error
	}
	done := make(chan result, 1)

	go func() {
		// Scan for ESC [ ... R. Anything else is input that arrived before the
		// reply and is dropped; the probe runs before the game reads keys, so
		// there is nothing worth keeping.
		for {
			b, err := reader.ReadByte()
			if err != nil {
				done <- result{0, err}
				return
			}
			if b != 0x1b {
				continue
			}
			if b, err = reader.ReadByte(); err != nil {
				done <- result{0, err}
				return
			}
			if b != '[' {
				continue
			}
			var body strings.Builder
			for {
				b, err = reader.ReadByte()
				if err != nil {
					done <- result{0, err}
					return
				}
				if b == 'R' {
					break
				}
				if body.Len() > 16 {
					done <- result{0, errors.New("cursor report too long")}
					return
				}
				body.WriteByte(b)
			}
			_, colText, ok := strings.Cut(body.String(), ";")
			if !ok {
				done <- result{0, fmt.Errorf("malformed cursor report %q", body.String())}
				return
			}
			col, err := strconv.Atoi(colText)
			if err != nil {
				done <- result{0, fmt.Errorf("malformed column %q: %w", colText, err)}
				return
			}
			done <- result{col, nil}
			return
		}
	}()

	select {
	case r := <-done:
		return r.col, r.err
	case <-deadline:
		// The reader goroutine is still blocked on a read that will never
		// return. VerifyGlyphWidths closes the terminal it opened as it
		// unwinds, which fails that read and lets the goroutine exit — and
		// because that descriptor is not stdin, the game's input is untouched
		// either way.
		return 0, errors.New("timed out waiting for cursor position report")
	}
}
