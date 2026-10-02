package tui

import (
	"fmt"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The Activity screen: a stacked area chart of what the colonists spend their
// time doing over the whole game — sleeping, eating, cooking, mining,
// fighting, fleeing — from the activity tallies on Snapshot.Population, with
// a legend of each activity's share now and over the game. Each band is
// split: the activity itself in its colour, and the time spent walking to it
// (fetching a meal, heading to the scumhouse to cook) in a darker shade just
// above. `c` switches the chart between shares of colonist time and average
// colonists. See
// docs/activity-screen.md.

// activityBand is one stacked band: which activity, its colour, and the
// darker shade of it for walking there.
type activityBand struct {
	act         sim.Activity
	color, walk lipgloss.Color
}

// activityBands is the stacking order, bottom to top: needs at the floor,
// work in the middle, danger above it, and idle as the lid, so a crisis reads
// as the red and magenta bands swelling up under the lid.
var activityBands = []activityBand{
	{sim.ActSleeping, "61", "60"},
	{sim.ActEating, "208", "130"},
	{sim.ActRelieving, "94", "58"},
	{sim.ActWashing, "117", "67"},
	{sim.ActSocializing, "213", "133"},
	{sim.ActCooking, "226", "142"},
	{sim.ActMining, "248", "243"},
	{sim.ActBuilding, "33", "25"},
	{sim.ActHauling, "37", "30"},
	{sim.ActCleaning, "113", "65"},
	{sim.ActEscaping, "137", "95"},
	{sim.ActFleeing, "201", "90"},
	{sim.ActFighting, "196", "88"},
	{sim.ActIdle, "239", "236"},
}

// segment i of the stack is band i/2: even segments the activity itself,
// odd ones walking to it.
func segmentColor(i int) lipgloss.Color {
	if i%2 == 1 {
		return activityBands[i/2].walk
	}
	return activityBands[i/2].color
}

// activityLegendWidth is the legend panel's width, borders included.
const activityLegendWidth = 36

func (m Model) handleActivityKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeMap
	case "c":
		m.activityCounts = !m.activityCounts
	}
	return m, nil
}

func (m Model) renderActivity() string {
	header := m.renderHeader()
	footer := m.footerLine("ACTIVITY  c shares/colonists  +/- speed  space pause  tab/esc map  q quit")
	rows := m.rosterRows()
	samples := m.latest.Population

	chartW := m.termW
	legendW := 0
	if m.termW >= 2*activityLegendWidth {
		legendW = activityLegendWidth
		chartW -= legendW
	}
	body := activityChart(samples, m.activityCounts, chartW, rows)
	if legendW > 0 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, activityLegend(samples, m.activityCounts, legendW, rows))
	}
	return strings.Join([]string{header, body, footer}, "\n")
}

// activitySlice is the tallies of a run of samples: colonist-ticks per
// activity, the part of them spent walking there, and the ticks they cover.
type activitySlice struct {
	n, walk [sim.NumActivities]int
	ticks   int
}

func (s activitySlice) total() int {
	t := 0
	for _, n := range s.n {
		t += n
	}
	return t
}

// value is how much of the slice activity a takes: its share of colonist
// time, or the average number of colonists doing it.
func (s activitySlice) value(a sim.Activity, counts bool) float64 {
	return s.scale(s.n[a], counts)
}

// scale turns colonist-ticks n into a share of the slice's colonist time, or
// an average number of colonists over it.
func (s activitySlice) scale(n int, counts bool) float64 {
	if counts {
		if s.ticks == 0 {
			return 0
		}
		return float64(n) / float64(s.ticks)
	}
	if t := s.total(); t > 0 {
		return float64(n) / float64(t)
	}
	return 0
}

// sliceOf sums samples[lo:hi]. Each sample's tally covers the ticks since the
// sample before it (or the start of the game).
func sliceOf(samples []sim.PopulationSample, lo, hi int) activitySlice {
	var s activitySlice
	for _, x := range samples[lo:hi] {
		for a, n := range x.Activity {
			s.n[a] += n
			s.walk[a] += x.Walking[a]
		}
	}
	prev := 0
	if lo > 0 {
		prev = samples[lo-1].Tick
	}
	s.ticks = samples[hi-1].Tick - prev
	return s
}

// activityColumns buckets the whole history into cols slices, one per plot
// column: a long history is summed down, a short one repeated across.
func activityColumns(samples []sim.PopulationSample, cols int) []activitySlice {
	out := make([]activitySlice, cols)
	n := len(samples)
	for x := range out {
		lo := x * n / cols
		hi := max(lo+1, (x+1)*n/cols)
		out[x] = sliceOf(samples, lo, hi)
	}
	return out
}

// activityChart draws the bordered stacked area chart, width by height cells.
func activityChart(samples []sim.PopulationSample, counts bool, width, height int) string {
	inner := panelInner(width)
	content := max(0, height-borderCells)
	title, unit := "COLONIST ACTIVITY", "share of colonist time"
	if counts {
		unit = "average colonists"
	}
	lines := []string{cells.Truncate(labelStyle.Render(title)+"  "+helpStyle.Render(unit), inner)}

	plotW := inner - perfLabelWidth - 1
	plotRows := content - 3 // title, axis rule, tick labels
	switch {
	case len(samples) == 0:
		lines = append(lines, "", helpStyle.Render(cells.Truncate("no samples yet", inner)))
	case plotRows >= 1 && plotW >= 1:
		cols := activityColumns(samples, plotW)
		lines = append(lines, stackPlot(cols, counts, plotW, plotRows)...)
		first, last := samples[0].Tick, samples[len(samples)-1].Tick
		lines = append(lines,
			strings.Repeat(" ", perfLabelWidth)+"└"+strings.Repeat("─", plotW),
			tickAxis(first, last, perfLabelWidth+1, plotW))
	}
	return sidebarStyle.Width(width - borderCells).Height(content).MaxHeight(height).
		Render(strings.Join(lines, "\n"))
}

// stackPlot draws rows lines of the stacked bands, each a y-axis label, the
// axis, and one cell per column. Each band is two segments, the activity and
// then walking to it. Each cell is two dots tall — an upper half block in the
// upper segment's colour on the lower segment's — which doubles the vertical
// resolution of plain blocks.
func stackPlot(cols []activitySlice, counts bool, plotW, rows int) []string {
	top := 1.0 // shares fill the plot
	if counts {
		top = 0
		for _, c := range cols {
			if c.ticks > 0 {
				top = max(top, float64(c.total())/float64(c.ticks))
			}
		}
		top = max(1, float64(int(top+0.999))) // a whole number of colonists
	}
	dots := rows * 2
	// band[x][d] is the segment at dot d (0 at the bottom) of column x, or
	// -1 above the stack.
	band := make([][]int, plotW)
	for x, c := range cols {
		band[x] = make([]int, dots)
		bounds := make([]float64, 2*len(activityBands))
		sum := 0.0
		for i, b := range activityBands {
			sum += c.scale(c.n[b.act]-c.walk[b.act], counts)
			bounds[2*i] = sum
			sum += c.scale(c.walk[b.act], counts)
			bounds[2*i+1] = sum
		}
		i := 0
		for d := range band[x] {
			v := (float64(d) + 0.5) / float64(dots) * top
			for i < len(bounds) && bounds[i] < v {
				i++
			}
			if i == len(bounds) {
				band[x][d] = -1
			} else {
				band[x][d] = i
			}
		}
	}

	out := make([]string, rows)
	every := max(1, rows/5)
	for r := 0; r < rows; r++ {
		label, axis := "", "│"
		if r == 0 || r == rows-1 || (r%every == 0 && rows-1-r >= every/2+1) {
			// The value at the top edge of this row, or the floor under the
			// bottom one.
			v := top * float64(rows-r) / float64(rows)
			if r == rows-1 {
				v = 0
			}
			if counts {
				label = fmt.Sprintf("%.1f", v)
			} else {
				label = fmt.Sprintf("%.0f%%", 100*v)
			}
			axis = "┤"
		}
		upper, lower := dots-1-2*r, dots-2-2*r
		out[r] = fmt.Sprintf("%*s", perfLabelWidth, cells.Truncate(label, perfLabelWidth)) +
			axis + halfBlockRow(band, upper, lower)
	}
	return out
}

// halfBlockRow renders one row of cells from two dot rows, styling each run
// of identical cells once rather than every cell.
func halfBlockRow(band [][]int, upper, lower int) string {
	var sb strings.Builder
	cell := func(u, l int) (string, lipgloss.Style) {
		st := lipgloss.NewStyle()
		switch {
		case u < 0 && l < 0:
			return " ", st
		case u == l:
			return "█", st.Foreground(segmentColor(u))
		case u < 0:
			return "▄", st.Foreground(segmentColor(l))
		case l < 0: // a band ending mid-cell with nothing below cannot happen in a stack, but stay safe
			return "▀", st.Foreground(segmentColor(u))
		default:
			return "▀", st.Foreground(segmentColor(u)).Background(segmentColor(l))
		}
	}
	for x := 0; x < len(band); {
		u, l := band[x][upper], band[x][lower]
		run := x + 1
		for run < len(band) && band[run][upper] == u && band[run][lower] == l {
			run++
		}
		glyph, st := cell(u, l)
		sb.WriteString(st.Render(strings.Repeat(glyph, run-x)))
		x = run
	}
	return sb.String()
}

// activityLegend lists every band, top of the stack first to match the chart,
// with its share over the latest sample and over the whole game (or, counting,
// the average colonists on it), and how much of its time over the game went
// on walking there. The swatch shows both shades.
func activityLegend(samples []sim.PopulationSample, counts bool, width, height int) string {
	inner := panelInner(width)
	content := max(0, height-borderCells)
	var now, game activitySlice
	if len(samples) > 0 {
		now = sliceOf(samples, len(samples)-1, len(samples))
		game = sliceOf(samples, 0, len(samples))
	}
	format := func(s activitySlice, a sim.Activity) string {
		if counts {
			return fmt.Sprintf("%5.1f", s.value(a, true))
		}
		return fmt.Sprintf("%4.0f%%", 100*s.value(a, false))
	}
	lines := []string{
		labelStyle.Render("LEGEND"),
		labelStyle.Render(fmt.Sprintf("%-14s %5s %5s %5s", "", "now", "game", "walk")),
	}
	for i := len(activityBands) - 1; i >= 0; i-- {
		b := activityBands[i]
		swatch := lipgloss.NewStyle().Foreground(b.color).Render("█") +
			lipgloss.NewStyle().Foreground(b.walk).Render("█")
		walk := "    -"
		if n := game.n[b.act]; n > 0 {
			walk = fmt.Sprintf("%4.0f%%", 100*float64(game.walk[b.act])/float64(n))
		}
		lines = append(lines, cells.Truncate(fmt.Sprintf("%s %-11s %s %s %s",
			swatch, b.act, format(now, b.act), format(game, b.act), walk), inner))
	}
	return sidebarStyle.Width(width - borderCells).Height(content).MaxHeight(height).
		Render(strings.Join(lines, "\n"))
}
