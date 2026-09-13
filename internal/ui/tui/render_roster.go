package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"

	"github.com/charmbracelet/lipgloss"
)

const rosterListWidth = 26

var (
	rosterSelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	labelStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	traitStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
)

// colonists returns the latest frame's colonists sorted by ID, so the roster
// order is stable frame to frame.
func (m Model) colonists() []sim.EntityView {
	if m.latest == nil {
		return nil
	}
	cs := make([]sim.EntityView, 0, len(m.latest.Entities))
	for _, e := range m.latest.Entities {
		if e.Kind == sim.Colonist {
			cs = append(cs, e)
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].ID < cs[j].ID })
	return cs
}

func (m Model) renderRoster() string {
	header := m.renderHeader()
	footer := helpStyle.Render("↑↓/jk select  tab/esc map  space pause  q quit")

	cs := m.colonists()
	if len(cs) == 0 {
		return strings.Join([]string{header, "No colonists in the colony.", footer}, "\n")
	}
	sel := clamp(m.selected, 0, len(cs)-1)

	rows := m.termH - headerRows - footerRows
	if rows < minRows {
		rows = minRows
	}

	list := m.renderColonistList(cs, sel, rows)
	detail := m.renderColonistDetail(cs[sel], rows)
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, " ", detail)
	return strings.Join([]string{header, body, footer}, "\n")
}

// renderColonistList draws the scrolling name column, keeping the selection in
// view.
func (m Model) renderColonistList(cs []sim.EntityView, sel, rows int) string {
	capacity := rows - 3 // box borders + the heading line
	if capacity < 1 {
		capacity = 1
	}
	start := 0
	if sel >= capacity {
		start = sel - capacity + 1
	}
	end := start + capacity
	if end > len(cs) {
		end = len(cs)
	}

	var b strings.Builder
	b.WriteString(labelStyle.Render(fmt.Sprintf("COLONISTS (%d)", len(cs))))
	b.WriteByte('\n')
	for i := start; i < end; i++ {
		name := colonistName(cs[i])
		line := truncate(name, rosterListWidth-4)
		if i == sel {
			b.WriteString(rosterSelStyle.Render("› " + line))
		} else {
			b.WriteString("  " + line)
		}
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return sidebarStyle.Width(rosterListWidth - 2).Height(rows - 2).Render(b.String())
}

// renderColonistDetail draws the inspector for one colonist: identity,
// attributes, health, needs, and traits.
func (m Model) renderColonistDetail(c sim.EntityView, rows int) string {
	width := m.termW - rosterListWidth - 3
	if width < 24 {
		width = 24
	}
	// Leave room in a bar line for the box (border+padding), the 7-wide label,
	// two spaces, and the "NNNN/NNNN" count so it never wraps.
	barW := width - 24
	if barW < 6 {
		barW = 6
	}
	if barW > 40 {
		barW = 40
	}

	var b strings.Builder
	p := c.Profile
	if p == nil {
		b.WriteString("(no profile)")
		return sidebarStyle.Width(width).Height(rows - 2).Render(b.String())
	}

	b.WriteString(titleStyle.Render(p.Name) + "\n")
	b.WriteString(statStyle.Render(fmt.Sprintf("%s · %s · %s", p.Gender, p.Sex, p.Orientation)) + "\n")
	b.WriteString(statStyle.Render(fmt.Sprintf("%d cm · %d kg", p.HeightCM, p.WeightKG)) + "\n\n")

	b.WriteString(labelStyle.Render("STATUS") + "  " + c.State.String() + "\n")
	b.WriteString(bar("health", c.HP, c.MaxHP, barW) + "\n\n")

	b.WriteString(labelStyle.Render("NEEDS") + "\n")
	for i := range c.Needs {
		meta := m.latest.NeedsMeta[i]
		name := meta.Name
		if meta.Fatal {
			name += "!"
		}
		b.WriteString(bar(name, c.Needs[i], meta.Max, barW) + "\n")
	}

	b.WriteString("\n" + labelStyle.Render("INVENTORY") + "\n")
	for i, stack := range c.Inventory {
		item := "empty"
		if stack.Count > 0 {
			item = fmt.Sprintf("%s ×%d", stack.Kind, stack.Count)
		}
		b.WriteString(statStyle.Render(fmt.Sprintf("  %d. %s", i+1, item)) + "\n")
	}

	b.WriteString("\n" + labelStyle.Render("TRAITS") + "\n")
	if len(p.Traits) == 0 {
		b.WriteString(statStyle.Render("  none — steady and average"))
	} else {
		lines := make([]string, 0, len(p.Traits))
		for _, tr := range p.Traits {
			lines = append(lines, traitStyle.Render("• "+tr.Name())+"\n  "+statStyle.Render(truncate(tr.Desc(), width-4)))
		}
		b.WriteString(strings.Join(lines, "\n"))
	}
	return sidebarStyle.Width(width).Height(rows - 2).Render(b.String())
}

// bar renders a labeled proportion bar like "health  ████····  30/40".
func bar(label string, val, max, width int) string {
	if max < 1 {
		max = 1
	}
	if val < 0 {
		val = 0
	}
	if val > max {
		val = max
	}
	if width < 1 {
		width = 1
	}
	filled := val * width / max
	gauge := strings.Repeat("█", filled) + strings.Repeat("·", width-filled)
	return fmt.Sprintf("%-7s %s %d/%d", truncate(label, 7), gauge, val, max)
}

// colonistName is the colonist's name, or a fallback if the profile is missing.
func colonistName(c sim.EntityView) string {
	if c.Profile != nil && c.Profile.Name != "" {
		return c.Profile.Name
	}
	return fmt.Sprintf("colonist #%d", c.ID)
}
