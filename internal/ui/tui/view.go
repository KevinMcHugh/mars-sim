package tui

import (
	"fmt"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	"github.com/charmbracelet/lipgloss"
)

// Layout constants. The map draws tileWidth terminal cells per tile; the
// sidebar is a fixed-width info panel to its right.
//
// sidebarWidth is the panel's *total* footprint including its border.
// lipgloss's Style.Width sets the content box — padding in, border out — so a
// panel is rendered at Width(sidebarWidth - borderCells) to land on
// sidebarWidth cells overall. TestPanelsRenderAtTheirDeclaredWidth pins this
// down; getting it wrong pushes the panel past the right edge, where the
// terminal wraps it and every row below shifts.
const (
	sidebarWidth = 30
	borderCells  = 2 // one column of box border on each side
	panelGap     = 1 // the single space between the map and the sidebar
	headerRows   = 2
	footerRows   = 1
	minCols      = 10
	minRows      = 6

	// minPanelWidth is the narrowest a bordered panel may be and still say
	// anything useful. A screen that cannot fit two panels at this width shows
	// one, rather than drawing both and letting the second run off the edge.
	minPanelWidth = 26
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	statStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	pausedStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	menuStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	logStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
)

// splitPanels divides the terminal between a list panel and a detail panel,
// in total cells including each panel's border.
//
// The list has a preferred width and the detail takes what is left. When the
// remainder is too narrow to be worth drawing, the detail is dropped (width 0)
// and the list keeps the space — the same call the map/sidebar split makes.
// Both list screens used to floor the detail panel at a minimum instead, which
// made the two panels add up to more than the terminal and pushed the
// inspector's right border off-screen.
func (m Model) splitPanels(preferredList int) (listWidth, detailWidth int) {
	listWidth = min(preferredList, m.termW)
	if listWidth < 1 {
		listWidth = 1
	}
	detailWidth = m.termW - listWidth - panelGap
	if detailWidth < minPanelWidth {
		return listWidth, 0
	}
	return listWidth, detailWidth
}

// sidebarFits reports whether the terminal is wide enough to show the map's
// minimum width and the sidebar side by side.
//
// Below this the sidebar is dropped rather than drawn anyway. The old code
// floored the map at minCols unconditionally and subtracted the sidebar from
// whatever was left, so a narrow terminal asked for more columns than it had:
// the sidebar was pushed past the right edge and the terminal clipped its
// border. Giving the map the full width is the honest answer — there is no
// arrangement that fits both.
func (m Model) sidebarFits() bool {
	return m.termW >= minCols*tileWidth+panelGap+sidebarWidth
}

// viewportTiles returns how many tiles (columns, rows) fit in the map area given
// the current terminal size, clamped to the world's dimensions.
func (m Model) viewportTiles() (cols, rows int) {
	availW := m.termW
	if m.sidebarFits() {
		availW -= sidebarWidth + panelGap
	}
	cols = availW / tileWidth
	rows = m.termH - headerRows - footerRows

	if cols < 1 {
		cols = 1
	}
	if rows < minRows {
		rows = minRows
	}
	if m.latest != nil {
		cols = min(cols, m.latest.Width)
		rows = min(rows, m.latest.Height)
	}
	return cols, rows
}

func (m Model) render() string { return m.clampFrame(m.renderFrame()) }

// renderFrame draws the current screen. It is expected to produce a frame that
// already fits the terminal; render then puts it through clampFrame.
//
// The split exists for the tests: TestFrameNeverExceedsTerminalWidth asserts
// that clampFrame is a no-op, so a renderer that produces an over-wide line
// fails the build instead of being quietly rescued by the safety net. A net
// that hides its own catches is how this class of bug survived last time.
func (m Model) renderFrame() string {
	if m.latest == nil || m.termW == 0 {
		return "Booting Mars colony simulation...\n"
	}

	var frame string
	switch m.mode {
	case modeRoster:
		frame = m.renderRoster()
	case modeJobs:
		frame = m.renderJobs()
	default:
		body := m.renderMap()
		if m.sidebarFits() {
			body = lipgloss.JoinHorizontal(lipgloss.Top, body, strings.Repeat(" ", panelGap), m.renderSidebar())
		}
		frame = strings.Join([]string{
			m.renderHeader(),
			body,
			m.renderFooter(),
		}, "\n")
	}
	return frame
}

// clampFrame is the last line of defence: it trims every line of the finished
// frame to the terminal width.
//
// A line wider than the terminal is the worst failure mode available, because
// the terminal wraps it and every row below is pushed down and misaligned for
// the rest of the session. Each renderer already fits its own content, so this
// should be a no-op — but "should be" is what produced the bug this guards
// against, and the cost of being sure is one pass over the frame.
//
// Lines are deliberately *not* padded out to the full width. Bubble Tea's
// renderer only emits an erase-to-end-of-line when it measures a line as
// narrower than the terminal, so leaving them short is what clears any stale
// cells a previous frame left behind.
func (m Model) clampFrame(frame string) string {
	if m.termW < 1 {
		return frame // no size negotiated yet; nothing to clamp against
	}
	lines := strings.Split(frame, "\n")
	for i, line := range lines {
		if cells.Width(line) > m.termW {
			lines[i] = cells.Truncate(line, m.termW)
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderHeader() string {
	s := m.latest
	title := titleStyle.Render(fitGlyph(glyphMars) + "MARS-SIM")
	sub := statStyle.Render("Mars Colony")

	state := fmt.Sprintf("tick %d  |  %d tps", s.Tick, s.TicksPerSecond)
	if s.Paused {
		state += "  |  " + pausedStyle.Render("PAUSED")
	}
	counts := statStyle.Render(strings.Join([]string{
		fmt.Sprintf("%s %d", fitGlyph(glyphColonist), s.Stats.Colonists),
		fmt.Sprintf("%s %d", fitGlyph(glyphAlien), s.Stats.Aliens),
		fmt.Sprintf("%s %d", fitGlyph(glyphCat), s.Stats.Cats),
		fmt.Sprintf("%s %d", fitGlyph(glyphMouse), s.Stats.Mice),
		fmt.Sprintf("%s %d", fitGlyph(glyphPod), s.Stats.Pods),
		fmt.Sprintf("%s %d", fitGlyph(glyphToilet), s.Stats.Toilets),
		fmt.Sprintf("%s %d", fitGlyph(glyphBed), s.Stats.Beds),
		fmt.Sprintf("rooms %d", s.Stats.Rooms),
		fmt.Sprintf("excavated %d", s.Stats.FloorDug),
	}, "  "))

	line1 := lipgloss.JoinHorizontal(lipgloss.Left, title, "  ", sub)
	line2 := lipgloss.JoinHorizontal(lipgloss.Left, statStyle.Render(state), "   ", counts)
	return cells.Truncate(line1, m.termW) + "\n" + cells.Truncate(line2, m.termW)
}

func (m Model) renderMap() string {
	cols, rows := m.viewportTiles()
	rowWidth := cols * tileWidth

	// Index entities by position for O(1) lookup while drawing; aliens win ties.
	occ := make(map[sim.Point]sim.EntityView, len(m.latest.Entities))
	for _, e := range m.latest.Entities {
		if cur, ok := occ[e.Pos]; ok && cur.Kind == sim.Alien {
			continue
		}
		occ[e.Pos] = e
	}

	var b strings.Builder
	var row strings.Builder
	for y := 0; y < rows; y++ {
		row.Reset()
		for x := 0; x < cols; x++ {
			p := m.cam.Add(x, y)
			if e, ok := occ[p]; ok {
				row.WriteString(entityGlyph(e))
			} else {
				row.WriteString(tileGlyph(m.latest.TileAt(p)))
			}
		}
		// Every row is forced to exactly cols*tileWidth cells. Each glyph is
		// already fitted, so this is belt and braces — but it is what confines
		// a width surprise to the row it happens on instead of letting it
		// shear the sidebar and everything below.
		b.WriteString(cells.Fit(row.String(), rowWidth))
		if y < rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m Model) renderSidebar() string {
	_, rows := m.viewportTiles()
	inner := sidebarWidth - borderCells - 2 // padding is one cell on each side

	// Two columns of legend entries, each entry "<glyph> <label>". The label
	// column is padded by cell count, not by %-Ns: fmt pads to a byte count,
	// which is off by two for every one of these lines.
	type entry struct{ symbol, label string }
	legendRows := [][2]entry{
		{{glyphColonist, "colonist"}, {glyphAlien, "alien"}},
		{{glyphCat, "cat"}, {glyphMouse, "mouse"}},
		{{glyphFleeing, "fleeing"}, {glyphTalking, "talking"}},
		{{glyphPod, "food pod"}, {glyphToilet, "toilet"}},
		{{glyphBed, "bunk"}, {glyphWall, "wall"}},
		{{glyphRock, "rock"}, {glyphFloor, "open"}},
	}
	column := inner / 2

	lines := []string{"LEGEND"}
	for _, r := range legendRows {
		left := cells.Fit(fitGlyph(r[0].symbol)+" "+r[0].label, column)
		right := fitGlyph(r[1].symbol) + " " + r[1].label
		lines = append(lines, cells.Fit(left+right, inner))
	}
	lines = append(lines, "", "LOG")
	legend := strings.Join(lines, "\n")

	// Fill the rest of the panel height with the most recent log lines.
	logLines := m.latest.Log
	room := rows - len(lines) - 2
	if room < 1 {
		room = 1
	}
	if len(logLines) > room {
		logLines = logLines[len(logLines)-room:]
	}
	wrapped := make([]string, 0, len(logLines))
	for _, l := range logLines {
		wrapped = append(wrapped, logStyle.Render(cells.Truncate(l, inner)))
	}

	content := legend + "\n" + strings.Join(wrapped, "\n")
	return sidebarStyle.Width(sidebarWidth - borderCells).Height(rows - borderCells).Render(content)
}

func (m Model) renderFooter() string {
	help := "space pause  +/- speed  s spawn  b build  ←↑↓→/hjkl pan  tab roster/jobs  q quit"
	if usingASCIIGlyphs() {
		// The player should know why the colony looks like a roguelike: the
		// terminal, not the game, chose this.
		help = "ASCII glyphs (terminal emoji widths disagreed)  |  " + help
	}
	return m.footerLine(help)
}

// footerLine renders the given help text, unless a spawn/build menu is open,
// in which case it renders that menu's prompt instead — on whichever screen
// the menu was opened from.
func (m Model) footerLine(help string) string {
	if prompt, ok := m.menuPrompt(); ok {
		return cells.Truncate(menuStyle.Render(prompt), m.termW)
	}
	return cells.Truncate(helpStyle.Render(help), m.termW)
}
