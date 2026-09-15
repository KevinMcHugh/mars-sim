package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"

	"github.com/charmbracelet/lipgloss"
)

// rosterListWidth leaves room for the age/gender glyph and a space (3), the
// longest generated name (17 columns), they/them (8), "age 80" (6), the
// longest state ("relieving", 9), three separators (9), and the selection
// marker (2), plus the panel chrome.
const rosterListWidth = 59

var (
	rosterSelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	labelStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	traitStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	kinStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
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
	footer := m.footerLine("↑↓/jk select  s spawn  b build  tab jobs  esc map  space pause  q quit")

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

// renderColonistList draws the scrolling name/status column, keeping the
// selection in view.
func (m Model) renderColonistList(cs []sim.EntityView, sel, rows int) string {
	capacity := (rows - 3) / 3 // box borders + heading, with three lines per colonist
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
		c := cs[i]
		name := colonistName(c)
		pronouns := "they/them"
		age := "age ?"
		state := c.State.String()
		if c.State == sim.Idle {
			state = "idling"
		}
		if c.Profile != nil {
			pronouns = c.Profile.Gender.Pronouns()
			if c.Profile.Age > 0 {
				age = fmt.Sprintf("age %d", c.Profile.Age)
			}
		}
		nameLine := truncate(colonistGlyph(c.Profile)+" "+name, rosterListWidth-4)
		infoLine := truncate(fmt.Sprintf("%s · %s", pronouns, age), rosterListWidth-4)
		stateLine := truncate(state, rosterListWidth-4)
		marker := "•"
		if i == sel {
			marker = "›"
			b.WriteString(rosterSelStyle.Render(marker + " " + nameLine))
		} else {
			b.WriteString(marker + " " + nameLine)
		}
		b.WriteByte('\n')
		b.WriteString("  " + infoLine + "\n")
		b.WriteString("  " + stateLine)
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	// MaxHeight matters here: selected-colonist details can contain a variable
	// number of memories and must not make the whole roster taller than the
	// terminal (which would push the header off-screen).
	return sidebarStyle.Width(rosterListWidth - 2).Height(rows - 2).MaxHeight(rows - 2).Render(b.String())
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

	b.WriteString(titleStyle.Render(colonistGlyph(p)+" "+p.Name) + "\n")
	b.WriteString(statStyle.Render(fmt.Sprintf("%s · %s · %s", p.Gender, p.Sex, p.Orientation)) + "\n")
	b.WriteString(statStyle.Render(fmt.Sprintf("age %d · %d cm · %d kg", p.Age, p.HeightCM, p.WeightKG)) + "\n")
	b.WriteString(statStyle.Render(fmt.Sprintf("%s skin · %s hair", p.SkinTone, p.HairColor)) + "\n\n")

	b.WriteString(labelStyle.Render("STATUS") + "  " + c.State.String() + "\n")
	b.WriteString(bar("health", c.HP, c.MaxHP, barW) + "\n")
	b.WriteString(moodLine(c.Mood, m.latest.MoodMax, barW) + "\n\n")

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
	used := 0
	for i, stack := range c.Inventory {
		if stack.Count == 0 {
			continue
		}
		used++
		b.WriteString(statStyle.Render(fmt.Sprintf("  %d. %s ×%d", i+1, stack.Kind, stack.Count)) + "\n")
	}
	if used == 0 {
		b.WriteString(statStyle.Render("  empty"))
	} else if used < len(c.Inventory) {
		b.WriteString(statStyle.Render(fmt.Sprintf("  %d empty slots", len(c.Inventory)-used)))
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

	names := m.colonistNames()
	b.WriteString("\n\n" + labelStyle.Render("FAMILY") + "\n")
	if len(c.Relations) == 0 {
		b.WriteString(statStyle.Render("  no known kin"))
	} else {
		lines := make([]string, 0, len(c.Relations))
		for _, rel := range c.Relations {
			line := fmt.Sprintf("• %s — %s", rel.Kind, names[rel.Other])
			lines = append(lines, kinStyle.Render(truncate(line, width-4)))
		}
		b.WriteString(strings.Join(lines, "\n"))
	}

	b.WriteString("\n\n" + labelStyle.Render("AFFINITIES") + "\n")
	if len(c.Affinities) == 0 {
		b.WriteString(statStyle.Render("  no acquaintances yet"))
	} else {
		lines := make([]string, 0, len(c.Affinities))
		for _, aff := range c.Affinities {
			lines = append(lines, affinityLine(names[aff.Other], aff.Value, m.latest.AffinityMax, barW))
		}
		b.WriteString(strings.Join(lines, "\n"))
	}

	b.WriteString("\n\n" + labelStyle.Render("MEMORIES") + "\n")
	if len(c.Memories) == 0 {
		b.WriteString(statStyle.Render("  no memories yet"))
	} else {
		start := len(c.Memories) - 5
		if start < 0 {
			start = 0
		}
		for _, memory := range c.Memories[start:] {
			line := fmt.Sprintf("  t%d: %s", memory.Tick, memory.Text)
			b.WriteString(statStyle.Render(truncate(line, width-4)) + "\n")
		}
	}
	return sidebarStyle.Width(width).Height(rows - 2).MaxHeight(rows - 2).Render(b.String())
}

// colonistNames maps colonist IDs to display names for the latest frame, so the
// inspector can name a colonist's relatives and acquaintances.
func (m Model) colonistNames() map[sim.EntityID]string {
	names := make(map[sim.EntityID]string)
	if m.latest == nil {
		return names
	}
	for _, e := range m.latest.Entities {
		if e.Kind == sim.Colonist {
			names[e.ID] = colonistName(e)
		}
	}
	return names
}

// affinityLine renders "Name  ···│██·  +40": a name, a diverging gauge that fills
// right for warmth and left for dislike, and the signed value.
func affinityLine(name string, val, max, width int) string {
	gaugeW := clamp(width, 3, 11)
	return fmt.Sprintf("%-12s %s %+d", truncate(name, 12), divergeGauge(val, max, gaugeW), val)
}

// moodLine renders the colonist's mood as a diverging gauge with a signed value
// and a one-word summary.
func moodLine(val, max, width int) string {
	gaugeW := clamp(width, 3, 13)
	return fmt.Sprintf("%-7s %s %+d %s", "mood", divergeGauge(val, max, gaugeW), val, moodWord(val, max))
}

// moodWord is a short label for a mood level, scaled to the mood range.
func moodWord(val, max int) string {
	if max < 1 {
		max = 1
	}
	pct := val * 100 / max
	switch {
	case pct >= 60:
		return "elated"
	case pct >= 20:
		return "content"
	case pct > -20:
		return "neutral"
	case pct > -60:
		return "glum"
	default:
		return "miserable"
	}
}

// divergeGauge renders a centered bar for a signed value: filled rightward from
// the axis for positives, leftward for negatives.
func divergeGauge(val, max, width int) string {
	if max < 1 {
		max = 1
	}
	if width < 3 {
		width = 3
	}
	val = clamp(val, -max, max)
	half := (width - 1) / 2
	mag := val
	if mag < 0 {
		mag = -mag
	}
	mag = mag * half / max
	left := make([]rune, half)
	right := make([]rune, half)
	for i := 0; i < half; i++ {
		left[i], right[i] = '·', '·'
	}
	if val < 0 {
		for i := 0; i < mag; i++ {
			left[half-1-i] = '█'
		}
	} else if val > 0 {
		for i := 0; i < mag; i++ {
			right[i] = '█'
		}
	}
	return string(left) + "│" + string(right)
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
