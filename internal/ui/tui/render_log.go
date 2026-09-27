package tui

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// logBandStyle stripes log entries so a wrapped entry stays one shade and the
// next entry is the other. The difference is deliberate small: enough to see
// where one event ends, not a second palette. The background is only on the
// odd band, which is what makes the stripe read as a bar once the line is
// padded out to the panel width.
var logBandStyle = [2]lipgloss.Style{
	lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
	lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Background(lipgloss.Color("236")),
}

// logRow is one terminal line of the colony log. entry is its index in the
// current snapshot's log; row is which wrapped line of that entry this is.
// Scrollback anchors to those two, not to a count of visual lines, because
// wrapping changes when the terminal is resized and the ring buffer drops
// entries from the front between frames.
type logRow struct {
	entry int
	row   int
	text  string
}

// logKindColumn is the type column's width, the widest LogKind label
// ("build complete"). A longer label fails TestLogKindLabelsFitTheColumn.
const logKindColumn = 14

// logKindGap separates the type column from the sentence.
const logKindGap = 2

// logEntriesDropped reports how many entries fell off the front of the ring
// between two snapshots. The snapshot replaces the slice; it does not say
// what was trimmed. The log only appends and drops from the front, so some
// suffix of prev is a prefix of next, and the shortest such drop is the one
// that happened. No overlap means a whole buffer of events arrived between
// frames (or the log was replaced): every old entry is gone.
func logEntriesDropped(prev, next []sim.LogEntry) int {
	if len(prev) == 0 {
		return 0
	}
	for drop := 0; drop <= len(prev); drop++ {
		remain := len(prev) - drop
		if remain > len(next) {
			continue
		}
		if slices.Equal(prev[drop:], next[:remain]) {
			return drop
		}
	}
	return len(prev)
}

// wrapLogEntry breaks one log event into lines of at most width cells,
// splitting on spaces and only cutting a word when it is wider than the
// panel. Nothing is marked with an ellipsis: a cut word looks like the
// sentence ended, which is the failure this exists to avoid.
func wrapLogEntry(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	if strings.TrimSpace(s) == "" {
		return []string{""}
	}
	var out []string
	for _, line := range wrapWords(s, width) {
		out = append(out, splitWide(line, width)...)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// splitWide hard-cuts s into pieces of at most width cells. wrapWords leaves
// a word that is itself too wide on its own line; drawing that line would
// push the panel past the terminal's right edge.
func splitWide(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	if s == "" || cells.Width(s) <= width {
		return []string{s}
	}
	var out []string
	for s != "" && cells.Width(s) > width {
		cut := ansi.Truncate(s, width, "")
		if cut == "" || len(cut) >= len(s) {
			// A cluster wider than the panel cannot be drawn. Drop it rather
			// than emit a line the frame cannot hold.
			_, size := utf8.DecodeRuneInString(s)
			if size == 0 {
				break
			}
			s = s[size:]
			continue
		}
		out = append(out, cut)
		s = s[len(cut):]
	}
	if s != "" {
		out = append(out, s)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// logColumns reports whether the type column fits, and how wide the sentence
// is beside it. A panel too narrow for the column plus a readable sentence
// drops the column: the sentence is the part that must not be squeezed into
// nothing. The map sidebar never asks; it is already that narrow.
func logColumns(inner int) (kindW, textW int, show bool) {
	const minText = 12
	if inner < logKindColumn+logKindGap+minText {
		if inner < 1 {
			inner = 1
		}
		return 0, inner, false
	}
	return logKindColumn, inner - logKindColumn - logKindGap, true
}

// logColumnHeader is the dim "type" / "event" line above the feed. It is
// empty when the panel is too narrow for the type column, so the header
// never names a column that is not there.
func logColumnHeader(inner int) string {
	kindW, _, show := logColumns(inner)
	if !show {
		return ""
	}
	gap := strings.Repeat(" ", logKindGap)
	line := cells.Pad("type", kindW) + gap + "event"
	return labelStyle.Render(cells.Pad(line, inner))
}

func logVisualRows(entries []sim.LogEntry, inner int) []logRow {
	kindW, textW, show := logColumns(inner)
	gap := strings.Repeat(" ", logKindGap)
	indent := strings.Repeat(" ", kindW) + gap
	var rows []logRow
	for i, entry := range entries {
		for r, line := range wrapLogEntry(entry.Text, textW) {
			text := line
			if show {
				if r == 0 {
					text = cells.Pad(entry.Kind.String(), kindW) + gap + line
				} else {
					text = indent + line
				}
			}
			rows = append(rows, logRow{entry: i, row: r, text: text})
		}
	}
	return rows
}

// logAnchorStart is the visual line where a scrollback anchor lands. A wrap
// row past the end of its entry (the terminal got wider) moves to the next
// entry rather than sitting on a row that no longer exists.
func logAnchorStart(rows []logRow, entry, row int) int {
	for i, r := range rows {
		if r.entry == entry && r.row >= row {
			return i
		}
		if r.entry > entry {
			return i
		}
	}
	if len(rows) == 0 {
		return 0
	}
	return len(rows) - 1
}

func (m Model) logBand(entry int) lipgloss.Style {
	return logBandStyle[(m.logBase+entry)&1]
}

func (m Model) paintLogLine(entry int, text string, inner int) string {
	return m.logBand(entry).Render(cells.Pad(text, inner))
}

// logLayout measures the log tab's feed: the wrapped lines, how many of them
// fit under the title, and whether the last row is a status line. It is
// derived from the latest frame and the terminal size, the same way the
// renderer derives them, so a key that arrives between frames scrolls the
// log the player is actually looking at.
func (m Model) logLayout() (visual []logRow, feed int, status bool) {
	width := m.termW
	if width < 1 {
		width = 1
	}
	var entries []sim.LogEntry
	if m.latest != nil {
		entries = m.latest.Log
	}
	visual = logVisualRows(entries, panelInner(width))
	contentH := m.rosterRows() - borderCells
	if contentH < 1 {
		contentH = 1
	}
	// The title is one row, and the column header is another when the type
	// column is showing. The status row is only spent when the feed does
	// not fit in what remains and a row is left to spend.
	chrome := 1
	if logColumnHeader(panelInner(width)) != "" {
		chrome++
	}
	feed = contentH - chrome
	if feed < 1 {
		return visual, 1, false
	}
	if len(visual) > feed {
		feed--
		if feed < 1 {
			return visual, 1, false
		}
		status = true
	}
	return visual, feed, status
}

// logWindow is the first visual line on screen. While the tab is following,
// that is the tail. While scrolled back, it is the anchored entry, clamped
// so the feed stays full.
func (m Model) logWindow(visual []logRow, feed int) (start int, live bool) {
	if feed < 1 {
		feed = 1
	}
	if len(visual) <= feed {
		return 0, true
	}
	maxStart := len(visual) - feed
	if !m.logScrolled {
		return maxStart, true
	}
	start = clamp(logAnchorStart(visual, m.logAnchor, m.logAnchorRow), 0, maxStart)
	return start, start >= maxStart
}

// scrollLog moves the viewport by delta visual lines. Reaching the tail
// resumes following, so the next event keeps the view on the live feed
// instead of leaving it one line behind forever.
func (m Model) scrollLog(delta int) Model {
	visual, feed, _ := m.logLayout()
	if len(visual) <= feed {
		m.logScrolled = false
		return m
	}
	maxStart := len(visual) - feed
	start := maxStart
	if m.logScrolled {
		start = clamp(logAnchorStart(visual, m.logAnchor, m.logAnchorRow), 0, maxStart)
	}
	start = clamp(start+delta, 0, maxStart)
	if start >= maxStart {
		m.logScrolled = false
		return m
	}
	m.logScrolled = true
	m.logAnchor = visual[start].entry
	m.logAnchorRow = visual[start].row
	return m
}

func (m Model) logPage() int {
	_, feed, _ := m.logLayout()
	if feed < 3 {
		return 1
	}
	return feed - 1
}

func (m Model) renderLog() string {
	header := m.renderHeader()
	footer := m.footerLine("LOG  ↑↓/jk scroll  pgup/pgdn page  home top  end live  tab/esc map  space pause  q quit")
	body := m.renderLogPanel(m.rosterRows(), m.termW)
	return strings.Join([]string{header, body, footer}, "\n")
}

func (m Model) renderLogPanel(rows, width int) string {
	if width < 1 {
		width = 1
	}
	inner := panelInner(width)
	contentH := rows - borderCells
	if contentH < 1 {
		contentH = 1
	}
	var entries []sim.LogEntry
	if m.latest != nil {
		entries = m.latest.Log
	}
	visual, feed, status := m.logLayout()
	start, live := m.logWindow(visual, feed)

	title := fmt.Sprintf("COLONY LOG%s%d", divider, len(entries))
	if live && !status {
		title += divider + "live"
	}

	var b strings.Builder
	b.WriteString(labelStyle.Render(cells.Truncate(title, inner)))
	if header := logColumnHeader(inner); header != "" {
		b.WriteByte('\n')
		b.WriteString(header)
	}
	if len(visual) == 0 {
		b.WriteByte('\n')
		b.WriteString(cells.Pad(labelStyle.Render("Nothing logged yet."), inner))
	} else {
		end := min(start+feed, len(visual))
		for _, row := range visual[start:end] {
			b.WriteByte('\n')
			b.WriteString(m.paintLogLine(row.entry, row.text, inner))
		}
		if status {
			for i := end - start; i < feed; i++ {
				b.WriteByte('\n')
			}
			b.WriteByte('\n')
			b.WriteString(logStatusLine(start, end, len(visual), inner, live))
		}
	}
	return sidebarStyle.Width(width - borderCells).Height(contentH).MaxHeight(rows).Render(b.String())
}

// logStatusLine tells the reader whether they are on the live tail or parked
// in the scrollback, and which slice of the wrapped log is on screen.
func logStatusLine(first, last, total, inner int, live bool) string {
	var line string
	if live {
		line = fmt.Sprintf("↑ %d earlier%slive", first, divider)
	} else {
		arrows := "↑↓"
		switch {
		case first == 0:
			arrows = " ↓"
		case last >= total:
			arrows = "↑ "
		}
		line = fmt.Sprintf("%s  %d-%d of %d  end live", arrows, first+1, last, total)
	}
	return labelStyle.Render(cells.Truncate(line, inner))
}

// sidebarLogLines is the tail of the colony log that fits in the map sidebar,
// newest entries preferred, each event wrapped rather than cut short. A
// single event taller than the room keeps its first lines: the start of the
// sentence is the part that says what happened.
func (m Model) sidebarLogLines(entries []sim.LogEntry, inner, room int) []string {
	if room < 1 || len(entries) == 0 {
		return nil
	}
	type block struct {
		entry int
		lines []string
	}
	var blocks []block
	used := 0
	for i := len(entries) - 1; i >= 0; i-- {
		wrapped := wrapLogEntry(entries[i].Text, inner)
		if used > 0 && used+len(wrapped) > room {
			break
		}
		if len(wrapped) > room-used {
			wrapped = wrapped[:room-used]
		}
		blocks = append(blocks, block{entry: i, lines: wrapped})
		used += len(wrapped)
		if used >= room {
			break
		}
	}
	out := make([]string, 0, used)
	for b := len(blocks) - 1; b >= 0; b-- {
		for _, line := range blocks[b].lines {
			out = append(out, m.paintLogLine(blocks[b].entry, line, inner))
		}
	}
	return out
}
