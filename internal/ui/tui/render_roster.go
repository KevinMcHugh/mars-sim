package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	"github.com/charmbracelet/lipgloss"
)

// rosterListWidth is the list panel's total footprint in cells, border
// included. It leaves room for the age/gender glyph and a space (3), the
// longest generated name (17 columns), they/them (8), "age 80" (6), the
// longest state ("relieving", 9), three separators (9), and the selection
// marker (2), plus the panel chrome.
const rosterListWidth = 59

// panelInner is the writable width inside a bordered panel of the given total
// width: the total less the border and the one cell of padding on each side.
func panelInner(total int) int {
	inner := total - borderCells - 2
	if inner < 1 {
		return 1
	}
	return inner
}

var (
	rosterSelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	labelStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	traitStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	kinStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
)

// rosterEntries returns the entities the roster currently shows, sorted by ID
// so the order is stable frame to frame. Colonists are always eligible;
// showNonHuman additionally admits aliens/cats/mice, and showDead further
// admits dead colonists (every one, from the permanent Snapshot.Deceased
// archive) and other recently-dead kinds (Snapshot.Graveyard, which is
// bounded and covers mice/cats/aliens too), subject to the same kind filter
// — a dead mouse only shows up once both filters are on. Graveyard also
// contains colonist records, but only Deceased is used for colonists here so
// one death is never listed twice. See docs/combat.md.
func (m Model) rosterEntries() []sim.EntityView {
	if m.latest == nil {
		return nil
	}
	include := func(kind sim.Kind) bool { return kind == sim.Colonist || m.showNonHuman }
	cs := make([]sim.EntityView, 0, len(m.latest.Entities))
	for _, e := range m.latest.Entities {
		if include(e.Kind) {
			cs = append(cs, e)
		}
	}
	if m.showDead {
		for _, e := range m.latest.Graveyard {
			if e.Kind != sim.Colonist && include(e.Kind) {
				cs = append(cs, e)
			}
		}
		for _, e := range m.latest.Deceased {
			cs = append(cs, e)
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].ID < cs[j].ID })
	return cs
}

// rosterTitle labels the list panel with the current count and, if either
// filter is on, which ones — so it's never a mystery why the list is longer
// than "just colonists".
func (m Model) rosterTitle(n int) string {
	title := fmt.Sprintf("ROSTER (%d)", n)
	var on []string
	if m.showDead {
		on = append(on, "dead")
	}
	if m.showNonHuman {
		on = append(on, "non-human")
	}
	if len(on) > 0 {
		title += " +" + strings.Join(on, " +")
	}
	return title
}

// rosterRows is the height, in terminal rows, that the roster's two panels
// get: everything the header and footer leave behind.
func (m Model) rosterRows() int {
	rows := m.termH - headerRows - footerRows
	if rows < minRows {
		rows = minRows
	}
	return rows
}

func (m Model) renderRoster() string {
	header := m.renderHeader()
	footer := m.footerLine("DETAILS  ↑↓/jk select  shift+↑↓ pgup/pgdn scroll  f filter  tab jobs  esc map  space pause  q quit")

	cs := m.rosterEntries()
	if len(cs) == 0 {
		empty := "No colonists in the colony."
		if m.showDead || m.showNonHuman {
			empty = "Nothing matches the current roster filter."
		}
		return strings.Join([]string{header, empty, footer}, "\n")
	}
	sel := clamp(m.selected, 0, len(cs)-1)

	rows := m.rosterRows()
	listWidth, detailWidth := m.splitPanels(rosterListWidth)
	body := m.renderColonistList(cs, sel, rows, listWidth)
	if detailWidth > 0 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body,
			strings.Repeat(" ", panelGap), m.renderColonistDetail(cs[sel], rows, detailWidth))
	}
	return strings.Join([]string{header, body, footer}, "\n")
}

// renderColonistList draws the scrolling name/status column, keeping the
// selection in view.
func (m Model) renderColonistList(cs []sim.EntityView, sel, rows, width int) string {
	inner := panelInner(width)
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
	b.WriteString(labelStyle.Render(m.rosterTitle(len(cs))))
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
		infoLine := c.Kind.String() // overwritten below for a colonist with a profile
		if c.Profile != nil {
			pronouns = c.Profile.Gender.Pronouns()
			if c.Profile.Age > 0 {
				age = fmt.Sprintf("age %d", c.Profile.Age)
			}
			infoLine = fmt.Sprintf("%s · %s", pronouns, age)
		}
		if c.Dead {
			state = "dead — " + c.Cause
		}
		// The glyph goes through fitGlyph here exactly as it does on the map.
		// The roster used to draw the bare constant, so a glyph the terminal
		// painted at an unexpected width shifted this column only. A
		// colonist's roster glyph deliberately ignores its transient State
		// (Fleeing, Fighting, ...) — the state line right below already says
		// that — but a non-colonist has no such "at rest" glyph, so it just
		// uses whatever entityGlyph draws for it.
		glyph := colonistGlyph(c.Profile)
		if c.Kind != sim.Colonist {
			glyph = entityGlyph(c)
		}
		nameLine := cells.Truncate(fitGlyph(glyph)+" "+name, inner-2)
		infoLine = cells.Truncate(infoLine, inner-2)
		stateLine := cells.Truncate(state, inner-2)
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
	// MaxHeight matters here: a panel taller than the terminal would push the
	// header off-screen. It is the block's height, border included — Height
	// sizes the content box but MaxHeight trims the finished block, so the
	// two are two cells apart. Passing the content height to both is what
	// used to eat the last row and the bottom border off every roster panel.
	return sidebarStyle.Width(width - borderCells).Height(rows - borderCells).MaxHeight(rows).Render(b.String())
}

// detailMetrics returns the inspector's writable width and the width of the
// gauges drawn inside it, for a panel of the given total width.
func detailMetrics(width int) (inner, barW int) {
	inner = panelInner(width)
	// Leave room in a bar line for the 7-wide label, two spaces, and the
	// "NNNN/NNNN" count so it never wraps.
	barW = inner - 20
	if barW < 6 {
		barW = 6
	}
	if barW > 40 {
		barW = 40
	}
	return inner, barW
}

// renderColonistDetail draws the inspector for one colonist: identity,
// attributes, health, needs, traits, family, affinities, and memories. The
// content routinely runs longer than the panel is tall, so it is windowed to
// the player's scroll position (see scrollDetail) rather than simply clipped.
func (m Model) renderColonistDetail(c sim.EntityView, rows, width int) string {
	inner, barW := detailMetrics(width)
	height := rows - borderCells
	lines := scrollDetail(m.detailLines(c, inner, barW), m.detailScroll, height, inner)
	// MaxHeight is belt and braces: scrollDetail already returns exactly
	// height lines, but a line the terminal measures wider than we do would
	// wrap and push the panel past the terminal's bottom, taking the header
	// with it. It counts the border, unlike Height — see renderColonistList.
	return sidebarStyle.Width(width - borderCells).Height(height).MaxHeight(height + borderCells).Render(strings.Join(lines, "\n"))
}

// detailLines builds the inspector's content as one string per terminal line,
// so the caller can window it. A line that runs long is left for scrollDetail
// to fit, which keeps the width work proportional to the rows on screen
// rather than to a colonist's whole history.
func (m Model) detailLines(c sim.EntityView, inner, barW int) []string {
	var b strings.Builder
	p := c.Profile
	if p == nil {
		return m.nonColonistDetailLines(c, inner, barW)
	}

	b.WriteString(titleStyle.Render(fitGlyph(colonistGlyph(p))+" "+p.Name) + "\n")
	b.WriteString(statStyle.Render(fmt.Sprintf("%s · %s", p.Gender.Pronouns(), p.Orientation)) + "\n")
	// Height reads in feet and inches first: mutation can stretch a colonist to
	// ten feet or shrink them to two (see docs/mutation.md), and that is a fact
	// about a person you want to take in at a glance, not convert in your head.
	b.WriteString(statStyle.Render(fmt.Sprintf("age %d · %s (%d cm) · %d kg",
		p.Age, sim.FormatHeight(p.HeightCM), p.HeightCM, p.WeightKG)) + "\n")
	b.WriteString(statStyle.Render(fmt.Sprintf("%s skin · %s hair", p.SkinTone, p.HairColor)) + "\n\n")

	status := c.State.String()
	if c.Dead {
		status = fmt.Sprintf("dead (tick %d) — %s", c.DiedTick, c.Cause)
	}
	b.WriteString(labelStyle.Render("STATUS") + "  " + status + "\n")
	b.WriteString(bar("health", c.HP, c.MaxHP, barW) + "\n")
	b.WriteString(moodLine(c.Charge, c.Grip, c.MoodLabel, barW) + "\n\n")

	// One compact line rather than a bar per part: six more full gauge lines
	// would crowd out everything below in the roster's fixed height (see
	// TestRosterShowsColonistDetail), and a wound only needs a number here,
	// not a gauge — the health bar above already gives the big picture.
	b.WriteString(labelStyle.Render("BODY") + "  " + cells.Truncate(strings.Join(bodyPartLines(c), "  "), inner-8) + "\n\n")

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
			lines = append(lines, traitStyle.Render("• "+tr.Name())+"\n  "+statStyle.Render(cells.Truncate(tr.Desc(), inner-2)))
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
			lines = append(lines, kinStyle.Render(cells.Truncate(line, inner-2)))
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
		// The whole remembered history, oldest first, not the last handful:
		// the panel scrolls now, so there is no reason to decide for the
		// player which memories are worth keeping on screen. A colonist holds
		// at most maxColonistMemories of them (see docs/memories.md).
		lines := make([]string, 0, len(c.Memories))
		for _, memory := range c.Memories {
			lines = append(lines, statStyle.Render(cells.Truncate("  "+memoryLine(memory), inner-2)))
		}
		b.WriteString(strings.Join(lines, "\n"))
	}
	return strings.Split(b.String(), "\n")
}

// memoryLine renders one memory the way the inspector lists it: the tick it
// happened on, then its text. A memory that stands for a run of the same minor
// event (see docs/memories.md) shows the span it covers and how many times it
// happened instead — "t1511-1630: Finished mining. (x12)" — so a repetitive
// stretch reads as one line without hiding when it started or when it ended.
func memoryLine(m sim.Memory) string {
	if m.Count > 1 {
		return fmt.Sprintf("t%d-%d: %s (x%d)", m.Tick, m.LastTick, m.Text, m.Count)
	}
	return fmt.Sprintf("t%d: %s", m.Tick, m.Text)
}

// scrollDetail windows content to a panel of the given height, starting at
// line off and fitting each visible line to the panel's width.
//
// Content that fits is drawn as-is. Content that doesn't gives its bottom row
// to a position line: without one, a panel that is merely clipped looks
// exactly like a panel that ends there, and the player has no way to know
// there is more to read or where in it they are.
func scrollDetail(lines []string, off, height, inner int) []string {
	if height < 1 {
		height = 1
	}
	if len(lines) <= height {
		out := make([]string, 0, len(lines))
		for _, line := range lines {
			out = append(out, cells.Truncate(line, inner))
		}
		return out
	}
	// Clamp here as well as in Update: the content shrinks on its own as a
	// colonist's state changes, and a stale offset would otherwise scroll the
	// panel past the end of its own text.
	off = clamp(off, 0, detailScrollMax(len(lines), height))
	end := min(off+height-1, len(lines))
	out := make([]string, 0, height)
	for _, line := range lines[off:end] {
		out = append(out, cells.Truncate(line, inner))
	}
	return append(out, scrollStatusLine(off, end, len(lines), inner))
}

// detailScrollMax is the furthest a panel of the given height can scroll
// through content of the given length: the offset that puts the last content
// line on the last row above the position line. Content that fits cannot
// scroll at all.
func detailScrollMax(total, height int) int {
	body := height - 1
	if body < 1 || total <= height {
		return 0
	}
	return total - body
}

// scrollStatusLine reports which slice of the content is on screen, and shows
// arrows only for the directions that actually have more to show.
func scrollStatusLine(first, last, total, inner int) string {
	arrows := "↑↓"
	switch {
	case first == 0:
		arrows = " ↓"
	case last >= total:
		arrows = "↑ "
	}
	line := fmt.Sprintf("%s  %d-%d of %d  shift+↑↓ scroll", arrows, first+1, last, total)
	return labelStyle.Render(cells.Truncate(line, inner))
}

// nonColonistDetailLines builds the inspector for an entity with no Profile:
// an alien, cat, mouse, or any dead entry lacking one. It has none of a
// colonist's needs/traits/family — just identity, status, and a body-part
// breakdown for the kinds that track one (Colonist and Alien; see
// docs/combat.md).
func (m Model) nonColonistDetailLines(c sim.EntityView, inner, barW int) []string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fitGlyph(entityGlyph(c))+" "+colonistName(c)) + "\n")
	b.WriteString(statStyle.Render(fmt.Sprintf("%s · #%d · (%d, %d)", c.Kind, c.ID, c.Pos.X, c.Pos.Y)) + "\n\n")

	if c.Dead {
		b.WriteString(labelStyle.Render("STATUS") + fmt.Sprintf("  dead (tick %d)\n", c.DiedTick))
		b.WriteString(statStyle.Render("  "+c.Cause) + "\n\n")
	} else {
		b.WriteString(labelStyle.Render("STATUS") + "  " + c.State.String() + "\n")
		b.WriteString(bar("health", c.HP, c.MaxHP, barW) + "\n\n")
	}

	if c.Kind == sim.Alien {
		b.WriteString(labelStyle.Render("BODY") + "  " + cells.Truncate(strings.Join(bodyPartLines(c), "  "), inner-8) + "\n")
	}
	return strings.Split(b.String(), "\n")
}

// bodyPartLines renders one "part cur/max" label per body part the entity
// actually has. A part with a zero maximum is one it does not have at all —
// every mutant part on anyone uranium has not changed (see docs/mutation.md) —
// and is skipped, so a mutant's third arm shows up here and nobody else grows
// a row of empty ones.
func bodyPartLines(c sim.EntityView) []string {
	out := make([]string, 0, len(c.Parts))
	for i, hp := range c.Parts {
		if c.MaxParts[i] == 0 {
			continue
		}
		out = append(out, fmt.Sprintf("%s %d/%d", sim.BodyPart(i).Short(), hp, c.MaxParts[i]))
	}
	return out
}

// colonistNames maps colonist IDs to display names for the latest frame, so the
// inspector can name a colonist's relatives and acquaintances — living or
// dead: Deceased is consulted too, so a family tree that reaches a dead
// relative still shows their name instead of a blank entry.
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
	for _, e := range m.latest.Deceased {
		names[e.ID] = colonistName(e)
	}
	return names
}

// affinityLine renders "Name  ···│██·  +40": a name, a diverging gauge that fills
// right for warmth and left for dislike, and the signed value.
func affinityLine(name string, val, max, width int) string {
	gaugeW := clamp(width, 3, 11)
	// cells.Fit, not %-12s: fmt pads to a byte count, so a name with any
	// non-ASCII in it would push the gauge out of column.
	return fmt.Sprintf("%s %s %+d", cells.Fit(name, 12), divergeGauge(val, max, gaugeW), val)
}

// moodLine exposes both affect axes and the cached contextual label on the
// single line formerly occupied by scalar mood, preserving roster height.
func moodLine(charge, grip int, label string, width int) string {
	line := fmt.Sprintf("%s C%+d G%+d %s", cells.Fit("affect", 7), charge, grip, label)
	return cells.Truncate(line, max(1, width+20))
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
	return fmt.Sprintf("%s %s %d/%d", cells.Fit(label, 7), gauge, val, max)
}

// colonistName is the colonist's name, or a fallback if the profile is missing.
func colonistName(c sim.EntityView) string {
	if c.Profile != nil && c.Profile.Name != "" {
		return c.Profile.Name
	}
	return fmt.Sprintf("%s #%d", c.Kind, c.ID)
}
