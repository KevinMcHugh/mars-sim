package tui

import (
	"slices"

	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"
)

// renderCache holds work carried between frames. Most of a frame is identical
// to the one before it — the sidebar changes only when the log does, and a tick
// moves a handful of entities across a handful of map rows — but measuring and
// styling it from scratch is where the render time goes.
//
// Model is copied by value through the Bubble Tea loop, so the cache lives
// behind a pointer every copy shares. Bubble Tea calls Update and View on a
// single goroutine (tea.Program.eventLoop), so it needs no lock.
//
// Every method tolerates a nil cache and simply does the work uncached, so a
// Model built without New still renders correctly.
type renderCache struct {
	sidebarValid bool
	sidebarRows  int
	sidebarASCII bool
	sidebarLog   []string
	sidebarOut   string

	// widths memoizes cells.Width for the lines of the previous frame. Width is
	// a pure function of the string, so a hit is exactly as correct as a scan.
	// Only the last frame's lines are kept (spare is recycled as the next
	// frame's map), which bounds the cache to one frame's worth of lines.
	widths map[string]int
	spare  map[string]int
}

func newRenderCache() *renderCache {
	return &renderCache{widths: map[string]int{}, spare: map[string]int{}}
}

// sidebar returns the memoized sidebar, calling draw only when an input it
// depends on has changed: the panel height, the event log, or the glyph set.
func (c *renderCache) sidebar(rows int, log []string, ascii bool, draw func() string) string {
	if c == nil {
		return draw()
	}
	if c.sidebarValid && c.sidebarRows == rows && c.sidebarASCII == ascii && slices.Equal(c.sidebarLog, log) {
		return c.sidebarOut
	}
	c.sidebarOut = draw()
	c.sidebarRows, c.sidebarASCII = rows, ascii
	c.sidebarLog = append(c.sidebarLog[:0], log...)
	c.sidebarValid = true
	return c.sidebarOut
}

// frameWidths measures the lines of one frame, reusing the previous frame's
// measurements for lines that have not changed.
type frameWidths struct {
	c *renderCache
}

// lineWidths starts measuring a frame. Call done when every line is measured.
func (c *renderCache) lineWidths() frameWidths {
	if c != nil {
		clear(c.spare)
	}
	return frameWidths{c}
}

// of returns the cell width of line.
func (f frameWidths) of(line string) int {
	if f.c == nil {
		return cells.Width(line)
	}
	w, ok := f.c.widths[line]
	if !ok {
		w = cells.Width(line)
	}
	f.c.spare[line] = w
	return w
}

// done makes this frame's measurements the ones the next frame reuses.
func (f frameWidths) done() {
	if f.c != nil {
		f.c.widths, f.c.spare = f.c.spare, f.c.widths
	}
}
