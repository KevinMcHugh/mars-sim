package tui

import (
	"fmt"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"

	"github.com/charmbracelet/lipgloss"
)

// Layout constants. The map draws two terminal cells per tile; the sidebar is a
// fixed-width info panel to its right.
const (
	sidebarWidth = 30
	headerRows   = 2
	footerRows   = 1
	minCols      = 10
	minRows      = 6
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	statStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	pausedStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	logStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
)

// viewportTiles returns how many tiles (columns, rows) fit in the map area given
// the current terminal size, clamped to the world's dimensions.
func (m Model) viewportTiles() (cols, rows int) {
	availW := m.termW - sidebarWidth - 1
	cols = availW / 2 // two cells per tile
	rows = m.termH - headerRows - footerRows

	if cols < minCols {
		cols = minCols
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

func (m Model) render() string {
	if m.latest == nil || m.termW == 0 {
		return "Booting Mars colony simulation...\n"
	}

	mapBlock := m.renderMap()
	sidebar := m.renderSidebar()
	body := lipgloss.JoinHorizontal(lipgloss.Top, mapBlock, " ", sidebar)

	return strings.Join([]string{
		m.renderHeader(),
		body,
		m.renderFooter(),
	}, "\n")
}

func (m Model) renderHeader() string {
	s := m.latest
	title := titleStyle.Render("\U0001F534 MARS-SIM")
	sub := statStyle.Render("Mars Colony")

	state := fmt.Sprintf("tick %d  |  %d tps", s.Tick, s.TicksPerSecond)
	if s.Paused {
		state += "  |  " + pausedStyle.Render("PAUSED")
	}
	counts := statStyle.Render(fmt.Sprintf(
		"\U0001F477 %d   \U0001F47D %d   \U0001F37D\uFE0F %d   \U0001F6BD %d   excavated %d",
		s.Stats.Colonists, s.Stats.Aliens, s.Stats.Pods, s.Stats.Toilets, s.Stats.FloorDug,
	))

	line1 := lipgloss.JoinHorizontal(lipgloss.Left, title, "  ", sub)
	line2 := lipgloss.JoinHorizontal(lipgloss.Left, statStyle.Render(state), "   ", counts)
	return line1 + "\n" + line2
}

func (m Model) renderMap() string {
	cols, rows := m.viewportTiles()

	// Index entities by position for O(1) lookup while drawing; aliens win ties.
	occ := make(map[sim.Point]sim.EntityView, len(m.latest.Entities))
	for _, e := range m.latest.Entities {
		if cur, ok := occ[e.Pos]; ok && cur.Kind == sim.Alien {
			continue
		}
		occ[e.Pos] = e
	}

	var b strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			p := m.cam.Add(x, y)
			if e, ok := occ[p]; ok {
				b.WriteString(entityGlyph(e))
			} else {
				b.WriteString(terrainGlyph(m.latest.TerrainAt(p)))
			}
		}
		if y < rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m Model) renderSidebar() string {
	_, rows := m.viewportTiles()

	legend := strings.Join([]string{
		"LEGEND",
		glyphColonist + " colonist   " + glyphAlien + " alien",
		glyphFleeing + " fleeing    " + glyphWall + " wall",
		glyphPod + " food pod   " + glyphToilet + " toilet",
		glyphRock + " rock       " + "   open",
		"",
		"LOG",
	}, "\n")

	// Fill the rest of the panel height with the most recent log lines.
	logLines := m.latest.Log
	room := rows - strings.Count(legend, "\n") - 3
	if room < 1 {
		room = 1
	}
	if len(logLines) > room {
		logLines = logLines[len(logLines)-room:]
	}
	wrapped := make([]string, 0, len(logLines))
	for _, l := range logLines {
		wrapped = append(wrapped, logStyle.Render(truncate(l, sidebarWidth-4)))
	}

	content := legend + "\n" + strings.Join(wrapped, "\n")
	return sidebarStyle.Width(sidebarWidth - 2).Height(rows - 2).Render(content)
}

func (m Model) renderFooter() string {
	return helpStyle.Render(
		"space pause  +/- speed  c colonist  a alien  \u2190\u2191\u2193\u2192/hjkl pan  q quit",
	)
}

func truncate(s string, n int) string {
	if n < 1 {
		n = 1
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "\u2026"
}
