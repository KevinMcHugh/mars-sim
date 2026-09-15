package tui

import (
	"bufio"
	"strings"
	"testing"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"
)

// The probe's parsing and arithmetic are testable without a terminal by
// replaying what a terminal would send. The raw-mode plumbing around them is
// not, so VerifyGlyphWidths itself is only covered by the not-a-TTY path below.

func TestReadCursorColumnParsesAReport(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int
	}{
		{"plain report", "\x1b[1;3R", 3},
		{"multi-digit column", "\x1b[24;137R", 137},
		{"leading keypresses are discarded", "qx\x1b[1;5R", 5},
		{"a non-CSI escape is skipped", "\x1bP0$r\x1b[1;7R", 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readCursorColumn(bufio.NewReader(strings.NewReader(tc.input)))
			if err != nil {
				t.Fatalf("readCursorColumn(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("readCursorColumn(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

// A terminal that does not answer must not hang the game: the probe gives up
// and the caller keeps the static widths.
func TestReadCursorColumnGivesUpOnSilence(t *testing.T) {
	start := time.Now()
	_, err := readCursorColumn(bufio.NewReader(blockingReader{}))
	if err == nil {
		t.Fatal("expected an error when the terminal never replies")
	}
	if elapsed := time.Since(start); elapsed > 2*probeTimeout {
		t.Errorf("took %v to give up, want roughly %v", elapsed, probeTimeout)
	}
}

func TestReadCursorColumnRejectsMalformedReports(t *testing.T) {
	for _, input := range []string{
		"\x1b[R",                                // no row;col at all
		"\x1b[1;notanumR",                       // column is not a number
		"\x1b[" + strings.Repeat("9", 40) + "R", // unbounded body
	} {
		if _, err := readCursorColumn(bufio.NewReader(strings.NewReader(input))); err == nil {
			t.Errorf("readCursorColumn(%q) should have failed", input)
		}
	}
}

// measureBatch turns a reported column into a width: the cursor starts in
// column 1, so a glyph that leaves it in column 3 painted two cells.
func TestMeasureConvertsColumnToWidth(t *testing.T) {
	var out strings.Builder
	reader := bufio.NewReader(strings.NewReader("\x1b[1;3R"))

	got, err := measureBatch(reader, &out, []string{"\U0001F477"})
	if err != nil {
		t.Fatalf("measureBatch: %v", err)
	}
	if len(got) != 1 || got[0] != 2 {
		t.Errorf("measureBatch reported %v, want [2]", got)
	}

	// The probe must leave no trace: park at column 1, draw, query, then return
	// to column 1 and erase the line.
	written := out.String()
	if !strings.HasPrefix(written, "\r") {
		t.Errorf("probe should park the cursor with \\r first, wrote %q", written)
	}
	if !strings.Contains(written, "\x1b[6n") {
		t.Errorf("probe should send a cursor position report request, wrote %q", written)
	}
	if !strings.HasSuffix(written, "\r\x1b[K") {
		t.Errorf("probe should erase what it drew, wrote %q", written)
	}
}

// A batch is one round trip: all the queries go out together and the replies
// come back in order. Getting the pairing wrong would silently attribute one
// glyph's width to another, which is worse than not probing at all.
func TestMeasureBatchPairsRepliesWithSymbols(t *testing.T) {
	symbols := []string{"\U0001F477", "ab", "\U0001F47D", "x"}
	// Columns are 1-based: widths 2, 2, 4 and 1.
	replies := "\x1b[1;3R\x1b[1;3R\x1b[1;5R\x1b[1;2R"

	var out strings.Builder
	got, err := measureBatch(bufio.NewReader(strings.NewReader(replies)), &out, symbols)
	if err != nil {
		t.Fatalf("measureBatch: %v", err)
	}
	want := []int{2, 2, 4, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("symbol %d (%+q): got %d cells, want %d", i, symbols[i], got[i], want[i])
		}
	}

	// Each symbol must be measured from column 1, so one unexpectedly wide
	// glyph cannot push the next one's reading along.
	written := out.String()
	for _, s := range symbols {
		if !strings.Contains(written, "\r"+s+"\x1b[6n") {
			t.Errorf("symbol %+q was not parked at column 1 before its query; wrote %q", s, written)
		}
	}
	if n := strings.Count(written, "\x1b[6n"); n != len(symbols) {
		t.Errorf("sent %d queries for %d symbols", n, len(symbols))
	}
}

// Every glyph the probe would measure has to survive the round trip, which
// means none of them may contain a control byte that would corrupt the escape
// sequence it is embedded in.
func TestEveryRegisteredGlyphIsSafeToProbe(t *testing.T) {
	for symbol := range glyphRegistry {
		for _, r := range symbol {
			if r < 0x20 || r == 0x7f {
				t.Errorf("glyph %+q contains control character %+q, which would corrupt the probe sequence", symbol, r)
			}
		}
		if cells.Width(symbol) < 1 && symbol != glyphFloor {
			t.Errorf("glyph %+q measures as zero cells; the probe cannot tell it from a failed write", symbol)
		}
	}
}

// blockingReader never returns data and never errors, standing in for a
// terminal that ignores the cursor position report.
type blockingReader struct{}

func (blockingReader) Read([]byte) (int, error) { select {} }
