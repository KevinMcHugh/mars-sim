package tui

import (
	"fmt"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"

	"github.com/charmbracelet/lipgloss"
)

// The Population screen: four braille line charts of the colony's vital signs
// over the whole game — colonists, meals in storage, colony size, fixtures —
// from Snapshot.Population, drawn with the Perf screen's chart (perfChart) on
// the simulation clock. See docs/population-screen.md.

var (
	popColonistStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("45"))
	popMealStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	popSizeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	popFixtureStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("177"))
)

// popSeries is one chart's worth: a title, a colour, and how to read a sample.
type popSeries struct {
	title string
	style lipgloss.Style
	value func(sim.PopulationSample) int
}

var popCharts = [4]popSeries{
	{"COLONISTS", popColonistStyle, func(s sim.PopulationSample) int { return s.Colonists }},
	{"MEALS IN STORAGE", popMealStyle, func(s sim.PopulationSample) int { return s.Meals }},
	{"COLONY SIZE (floor tiles)", popSizeStyle, func(s sim.PopulationSample) int { return s.ColonySize }},
	{"FIXTURES", popFixtureStyle, func(s sim.PopulationSample) int { return s.Fixtures }},
}

func (m Model) renderPopulation() string {
	header := m.renderHeader()
	footer := m.footerLine("POPULATION  +/- speed  space pause  tab/esc map  s spawn  b build  q quit")
	rows := m.rosterRows()
	samples := m.latest.Population

	// Two by two: each chart gets half the width and half the height.
	leftW := m.termW / 2
	rightW := m.termW - leftW
	topH := rows / 2
	bottomH := rows - topH
	chart := func(i, width, height int) string {
		return popChart(popCharts[i], samples, width).render(width, height)
	}
	top := lipgloss.JoinHorizontal(lipgloss.Top, chart(0, leftW, topH), chart(1, rightW, topH))
	bottom := lipgloss.JoinHorizontal(lipgloss.Top, chart(2, leftW, bottomH), chart(3, rightW, bottomH))
	return strings.Join([]string{header, top, bottom, footer}, "\n")
}

// popChart builds one chart panel width cells wide, stretching the whole
// history across its plot.
func popChart(s popSeries, samples []sim.PopulationSample, width int) perfChart {
	plotW := max(1, panelInner(width)-perfLabelWidth-1)
	c := perfChart{title: s.title, style: s.style, counts: true}
	if len(samples) == 0 {
		c.stats = []string{"no samples yet"}
		return c
	}
	c.values = stretch(samples, 2*plotW, s.value)
	lo, hi := s.value(samples[0]), s.value(samples[0])
	for _, x := range samples {
		lo, hi = min(lo, s.value(x)), max(hi, s.value(x))
	}
	c.stats = []string{
		fmt.Sprintf("now %d", s.value(samples[len(samples)-1])),
		fmt.Sprintf("min %d", lo),
		fmt.Sprintf("max %d", hi),
	}
	first, last := samples[0].Tick, samples[len(samples)-1].Tick
	c.xAxis = func(offset, plotW int) string { return tickAxis(first, last, offset, plotW) }
	return c
}

// stretch resamples samples to exactly cols points, so a short history fills
// the plot as steps and a long one is thinned: the chart always spans the
// whole game.
func stretch(samples []sim.PopulationSample, cols int, value func(sim.PopulationSample) int) []float64 {
	out := make([]float64, cols)
	for x := range out {
		out[x] = float64(value(samples[x*len(samples)/cols]))
	}
	return out
}

// tickAxis labels the first, middle and last tick under a plot that begins at
// column offset and is plotW cells wide, dropping a label that would collide
// with its neighbour.
func tickAxis(first, last, offset, plotW int) string {
	width := offset + plotW
	buf := []rune(strings.Repeat(" ", width))
	next := 0
	put := func(col, tick int) {
		label := []rune(fmt.Sprintf("t%d", tick))
		col = clamp(col, 0, width-len(label))
		if col < next || col < 0 {
			return
		}
		copy(buf[col:], label)
		next = col + len(label) + 1
	}
	put(offset, first)
	mid := fmt.Sprintf("t%d", (first+last)/2)
	put(offset+plotW/2-len(mid)/2, (first+last)/2)
	put(width-len(fmt.Sprintf("t%d", last)), last)
	return strings.TrimRight(string(buf), " ")
}
