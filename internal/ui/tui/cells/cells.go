// Package cells is the single source of truth for how wide a string is on a
// terminal, measured in character cells (columns).
//
// Every layout decision in the TUI — how many tiles fit, where the sidebar
// starts, how far to truncate a log line — is arithmetic on cell counts. Doing
// that arithmetic with len(), len([]rune(...)), or fmt's %-Ns padding is wrong
// for anything but ASCII, and the whole UI is built out of emoji. A single
// off-by-one cell shears every line below it and leaves stale cells on screen,
// because Bubble Tea's renderer only emits an erase-to-end-of-line when it
// believes a line is shorter than the terminal is wide.
//
// So: Width here is ansi.StringWidth, the exact function bubbletea's standard
// renderer and lipgloss.Width use internally. Agreeing with the renderer is not
// a nice-to-have — if our math and the renderer's math disagree, the renderer
// skips the clear and the previous frame bleeds through. Whether that shared
// number also matches the terminal's painted width is a separate question,
// answered by the glyph registry and the startup probe (see glyphs.go).
package cells

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Ellipsis marks a string that Truncate shortened. It is one cell wide.
const Ellipsis = "…"

// Width returns the number of terminal cells s occupies, ignoring any ANSI
// escape sequences (styled text measures as the text it paints). For
// multi-line input it returns the width of the widest line.
func Width(s string) int {
	if !strings.ContainsRune(s, '\n') {
		return ansi.StringWidth(s)
	}
	widest := 0
	for _, line := range strings.Split(s, "\n") {
		if w := ansi.StringWidth(line); w > widest {
			widest = w
		}
	}
	return widest
}

// Truncate shortens s to at most n cells, marking the cut with an ellipsis. It
// never splits a grapheme cluster, and never returns more than n cells: if the
// last cluster that fits is two cells wide and only one cell is left, that
// cluster is dropped rather than half-drawn.
func Truncate(s string, n int) string {
	if n < 1 {
		n = 1
	}
	if Width(s) <= n {
		return s
	}
	if n == 1 {
		return Ellipsis
	}
	return ansi.Truncate(s, n, Ellipsis)
}

// Pad right-pads s with spaces to exactly n cells. A string already at least n
// cells wide is returned unchanged; use Fit when the result must not exceed n.
func Pad(s string, n int) string {
	w := Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

// Fit returns s in exactly n cells, truncating what is too long and padding
// what is too short. Rendering through Fit is what keeps a width surprise
// local: a glyph the terminal paints a cell wider or narrower than we expect
// corrupts its own line and nothing else.
//
// One case cannot be squared: truncating before a wide cluster can leave a
// single cell that no ellipsis fits into, so Fit pads it with a space.
func Fit(s string, n int) string {
	if n < 1 {
		return ""
	}
	return Pad(Truncate(s, n), n)
}
