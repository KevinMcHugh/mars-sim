package tui

import (
	"fmt"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	"github.com/charmbracelet/lipgloss"
)

// loreListWidth is the list panel's total footprint in cells, border
// included: enough for the world-facts block and a species' short roster
// label ("Xeno · hostile") without wrapping.
const loreListWidth = 36

// renderLore draws the lore tab: world facts (size, how much has been
// explored, the seed) plus every rolled alien species, list-and-detail like
// the storage tab. See docs/lore.md.
func (m Model) renderLore() string {
	header := m.renderHeader()
	footer := m.footerLine("LORE  ↑↓/jk select species  tab/esc map  s spawn  b build  space pause  q quit")
	rows := m.rosterRows()

	listWidth, detailWidth := m.splitPanels(loreListWidth)
	body := m.renderLoreList(rows, listWidth)
	if detailWidth > 0 && len(m.latest.AlienSpecies) > 0 {
		sel := clamp(m.loreSelected, 0, len(m.latest.AlienSpecies)-1)
		body = lipgloss.JoinHorizontal(lipgloss.Top, body,
			strings.Repeat(" ", panelGap),
			m.renderLoreDetail(m.latest.AlienSpecies[sel], rows, detailWidth))
	}
	return strings.Join([]string{header, body, footer}, "\n")
}

// worldFactLines renders the handful of facts about the world itself, above
// the species list. Explored tracks Stats.ExploredTiles, which only moves
// while fog of war is on (see Snapshot.FogOfWar) -- with it off, every tile
// is already visible, so the line says that instead of a stuck 0%.
func (m Model) worldFactLines() []string {
	s := m.latest
	area := s.Width * s.Height
	lines := []string{
		fmt.Sprintf("Size: %d x %d (%d tiles)", s.Width, s.Height, area),
	}
	if !s.FogOfWar {
		lines = append(lines, "Explored: 100% (fog off)")
	} else {
		pct := 0
		if area > 0 {
			pct = s.Stats.ExploredTiles * 100 / area
		}
		lines = append(lines, fmt.Sprintf("Explored: %d%% (%d/%d tiles)", pct, s.Stats.ExploredTiles, area))
	}
	lines = append(lines, fmt.Sprintf("Seed: %d", s.Seed))
	return lines
}

func (m Model) renderLoreList(rows, width int) string {
	inner := panelInner(width)
	s := m.latest

	var b strings.Builder
	b.WriteString(labelStyle.Render("LORE · WORLD"))
	b.WriteByte('\n')
	for _, line := range m.worldFactLines() {
		b.WriteString(cells.Truncate(line, inner))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(labelStyle.Render(fmt.Sprintf("ALIEN SPECIES (%d)", len(s.AlienSpecies))))
	if len(s.AlienSpecies) == 0 {
		b.WriteString("\n")
		b.WriteString(cells.Truncate("None rolled.", inner))
		return sidebarStyle.Width(width - borderCells).Height(rows - borderCells).MaxHeight(rows).Render(b.String())
	}

	sel := clamp(m.loreSelected, 0, len(s.AlienSpecies)-1)
	for i, sp := range s.AlienSpecies {
		b.WriteByte('\n')
		marker := "•"
		if i == sel {
			marker = "›"
		}
		line := fmt.Sprintf("%s %d. %s", marker, i+1, sp.RosterLabel())
		if i == sel {
			b.WriteString(rosterSelStyle.Render(cells.Truncate(line, inner)))
		} else {
			b.WriteString(cells.Truncate(line, inner))
		}
	}
	return sidebarStyle.Width(width - borderCells).Height(rows - borderCells).MaxHeight(rows).Render(b.String())
}

// renderLoreDetail lists everything known about one rolled species: its
// build as explicit stat lines (for scanning at a glance) plus
// AlienSpecies.Description()'s narrative paragraph (for reading).
func (m Model) renderLoreDetail(sp sim.AlienSpecies, rows, width int) string {
	inner := panelInner(width)

	var b strings.Builder
	b.WriteString(titleStyle.Render(sp.RosterLabel()))
	b.WriteString("\n\n")

	stat := func(label, val string) {
		b.WriteString(cells.Truncate(fmt.Sprintf("%-14s %s", label, val), inner))
		b.WriteByte('\n')
	}
	stat("Height:", fmt.Sprintf("%d-%d cm", sp.HeightMinCM, sp.HeightMaxCM))
	stat("Weight:", fmt.Sprintf("%d-%d kg", sp.WeightMinKG, sp.WeightMaxKG))
	stat("Eyes:", fmt.Sprintf("%d", sp.Eyes))
	stat("Limbs:", fmt.Sprintf("%d (%d arms, %d legs)", sp.Limbs, sp.Arms, sp.Legs()))
	tail := "no"
	if sp.Tail {
		tail = "yes"
	}
	stat("Tail:", tail)
	stat("Skin:", sp.Skin.String())
	stat("Color:", sp.Color)
	stat("Pattern:", sp.Pattern.String())
	stat("Bite damage:", fmt.Sprintf("%d", sp.BiteDamage))
	stat("Bite cooldown:", fmt.Sprintf("%d ticks", sp.BiteRest))
	stat("Move pace:", fmt.Sprintf("every %d ticks", sp.Slowness))

	b.WriteString("\n")
	b.WriteString(labelStyle.Render("FIELD NOTES"))
	b.WriteByte('\n')
	for _, line := range wrapWords(sp.Description(), inner) {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return sidebarStyle.Width(width - borderCells).Height(rows - borderCells).MaxHeight(rows).Render(b.String())
}

// wrapWords greedily wraps s into lines of at most width cells, breaking
// only at spaces. A species Description is plain, single-paragraph text, so
// this does not need to handle anything more exotic than that.
func wrapWords(s string, width int) []string {
	if width < 1 {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	lines := make([]string, 0, 4)
	line := words[0]
	for _, w := range words[1:] {
		candidate := line + " " + w
		if cells.Width(candidate) > width {
			lines = append(lines, line)
			line = w
			continue
		}
		line = candidate
	}
	return append(lines, line)
}
