package tui

import (
	"fmt"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	"github.com/charmbracelet/lipgloss"
)

// The flow-field overlay: the map screen tints every tile a shared flow field
// reached by its distance to the field's nearest goal, and writes the distance
// on bare floor. `f` cycles through the fields the engine reports
// (Snapshot.FlowFields) and then back to off; `F` turns it off. See
// docs/flow-field-view.md.

// flowGoalStyle marks distance 0: the tiles a seeker stops on.
var flowGoalStyle = lipgloss.NewStyle().Background(lipgloss.Color("46")).Foreground(lipgloss.Color("0"))

// flowRamp shades distances 1..Max from near (green) to far (dark red). The
// colours are dark enough for an emoji drawn on top to stay legible.
var flowRamp = func() []lipgloss.Style {
	colors := []string{"28", "64", "100", "136", "130", "124", "88", "52"}
	styles := make([]lipgloss.Style, len(colors))
	for i, c := range colors {
		styles[i] = lipgloss.NewStyle().Background(lipgloss.Color(c)).Foreground(lipgloss.Color("252"))
	}
	return styles
}()

// flowBand is the ramp index for distance d (>= 1) on a field whose largest
// distance is max.
func flowBand(d, max int32) int {
	if max <= 1 {
		return 0
	}
	return min(int((d-1)*int32(len(flowRamp))/max), len(flowRamp)-1)
}

// flowStyle is how a tile at distance d is painted.
func flowStyle(d, max int32) lipgloss.Style {
	if d == 0 {
		return flowGoalStyle
	}
	return flowRamp[flowBand(d, max)]
}

// flowLabel is the distance written on a bare floor tile, exactly tileWidth
// cells wide. Past two digits it is left blank: the tint still says how far.
func flowLabel(d int32) string {
	if d >= 100 {
		return strings.Repeat(" ", tileWidth)
	}
	return fmt.Sprintf("%*d", tileWidth, d)
}

// shownFlowField is the field the overlay should draw: the snapshot's, if the
// overlay is on and the engine has caught up with the field last picked. A
// frame published before the engine heard the command draws nothing rather
// than a field the player has already cycled past.
func (m Model) shownFlowField() *sim.FlowFieldView {
	if !m.flowOn || m.latest == nil {
		return nil
	}
	if v := m.latest.FlowField; v != nil && v.Field == m.flowRef {
		return v
	}
	return nil
}

// cycleFlowField moves the overlay to the next field the engine offers, or
// off after the last one, and tells the engine which field to publish.
func (m *Model) cycleFlowField() {
	if m.latest == nil || len(m.latest.FlowFields) == 0 {
		return
	}
	refs := m.latest.FlowFields
	next := 0
	if m.flowOn {
		next = len(refs) // not found: off
		for i, r := range refs {
			if r == m.flowRef {
				next = i + 1
				break
			}
		}
	}
	if next >= len(refs) {
		m.hideFlowField()
		return
	}
	m.flowOn, m.flowRef = true, refs[next]
	m.send(sim.ShowFlowField{Show: true, Field: m.flowRef})
}

// hideFlowField turns the overlay off and stops the engine copying the field.
func (m *Model) hideFlowField() {
	if !m.flowOn {
		return
	}
	m.flowOn = false
	m.send(sim.ShowFlowField{})
}

// send forwards a command to the engine, if there is one (tests build a Model
// without).
func (m Model) send(cmd sim.Command) {
	if m.eng != nil {
		m.eng.Send(cmd)
	}
}

// flowTile paints one explored tile for the overlay. drawn is what the map
// would show there, and bare says it is empty floor, which gets the distance
// written on it instead.
func flowTile(v *sim.FlowFieldView, p sim.Point, drawn string, bare bool) string {
	d := v.At(p)
	if d < 0 {
		return drawn
	}
	if bare {
		drawn = flowLabel(d)
	}
	return flowStyle(d, v.Max).Render(drawn)
}

// renderFlowSidebar replaces the map legend while the overlay is on: which
// field is shown, its extent, and what the colours mean.
func (m Model) renderFlowSidebar(v *sim.FlowFieldView) string {
	_, rows := m.viewportTiles()
	inner := sidebarWidth - borderCells - 2

	lines := []string{"FLOW FIELD", m.flowRef.Name()}
	switch {
	case v == nil:
		lines = append(lines, "", "waiting for the engine…")
	case v.Goals == 0:
		lines = append(lines, "", "no goal tiles: no one", "routes by this field", "(a facility's leads only", "to fixtures everyone", "may use)")
	default:
		lines = append(lines,
			"",
			fmt.Sprintf("goal tiles   %d", v.Goals),
			fmt.Sprintf("farthest     %d steps", v.Max),
			"",
			flowGoalStyle.Render(strings.Repeat(" ", tileWidth))+" goal (distance 0)",
		)
		// One row per ramp band that holds any distance, with the range it
		// covers on this field.
		for b := range flowRamp {
			lo, hi := int32(-1), int32(-1)
			for d := int32(1); d <= v.Max; d++ {
				if flowBand(d, v.Max) == b {
					if lo < 0 {
						lo = d
					}
					hi = d
				}
			}
			if lo < 0 {
				continue
			}
			span := fmt.Sprintf("%d", lo)
			if hi != lo {
				span = fmt.Sprintf("%d–%d", lo, hi)
			}
			lines = append(lines, flowRamp[b].Render(strings.Repeat(" ", tileWidth))+" "+span+" steps")
		}
		lines = append(lines, "", "untinted floor can't", "reach a goal")
	}
	if i, n := m.flowIndex(); n > 0 {
		lines = append(lines, "", fmt.Sprintf("field %d of %d", i+1, n))
	}
	lines = append(lines, "f next field  F off")
	for i, l := range lines {
		lines[i] = cells.Fit(l, inner)
	}
	return sidebarStyle.Width(sidebarWidth - borderCells).Height(rows - borderCells).Render(strings.Join(lines, "\n"))
}

// flowIndex is the shown field's position in the engine's list, and the
// list's length.
func (m Model) flowIndex() (int, int) {
	if m.latest == nil {
		return 0, 0
	}
	for i, r := range m.latest.FlowFields {
		if r == m.flowRef {
			return i, len(m.latest.FlowFields)
		}
	}
	return 0, len(m.latest.FlowFields)
}

// flowCursorLabel describes the overlay's distance at the inspect cursor, for
// the footer, or "" with the overlay off.
func (m Model) flowCursorLabel() string {
	if !m.flowOn {
		return ""
	}
	v := m.shownFlowField()
	if v == nil || !m.latest.ExploredAt(m.cursor) {
		return ""
	}
	if d := v.At(m.cursor); d >= 0 {
		return fmt.Sprintf("  %s %d steps", m.flowRef.Name(), d)
	}
	return fmt.Sprintf("  %s unreachable", m.flowRef.Name())
}
