package tui

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	"github.com/charmbracelet/lipgloss"
)

// The Perf screen: two gping-style braille line charts, ticks per second on
// top and milliseconds per tick below, drawn from Snapshot.Perf. See
// docs/perf-screen.md.

const (
	// perfLabelWidth is the y-axis label column. It is fixed rather than
	// measured so the plot does not jitter sideways as the range changes;
	// "1234.5ms" and "123456" both fit.
	perfLabelWidth = 9

	// perfTPSWindow is how many samples one ticks-per-second point averages
	// over: one second's worth. A single quarter-second bucket at 8 tps holds
	// two ticks, or one, or three, and the graph would read as noise.
	perfTPSWindow = int(time.Second / sim.PerfBucket)
)

var (
	perfTPSStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	perfMsStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

func (m Model) renderPerf() string {
	header := m.renderHeader()
	footer := m.footerLine("PERF  +/- speed  space pause  tab/esc map  s spawn  b build  q quit")
	rows := m.rosterRows()
	top := rows / 2

	samples := m.latest.Perf
	inner := panelInner(m.termW)
	n := perfWindow(inner)
	tps := tail(tpsSeries(samples), n)
	ms := tail(msSeries(samples), n)
	shown := tail(samples, n)

	var start time.Time
	if len(shown) > 0 {
		start = shown[0].Start
	}
	span := time.Duration(n) * sim.PerfBucket

	tpsChart := perfChart{
		title: "TICKS/SEC", style: perfTPSStyle, values: tps, start: start, span: span,
		stats: tpsStats(tps, m.latest.TicksPerSecond),
		unit:  "",
	}
	msChart := perfChart{
		title: "MS/TICK", style: perfMsStyle, values: ms, start: start, span: span,
		stats: msStats(shown, ms, m.latest.TicksPerSecond),
		unit:  "ms",
	}
	body := tpsChart.render(m.termW, top) + "\n" + msChart.render(m.termW, rows-top)
	return strings.Join([]string{header, body, footer}, "\n")
}

// perfWindow is how many samples fit across a chart whose panel content is
// inner cells wide: two per plot cell, since a braille cell is two dots wide.
func perfWindow(inner int) int {
	return max(1, 2*(inner-perfLabelWidth-1))
}

func tail[T any](s []T, n int) []T {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// tpsSeries turns bucket tick counts into a ticks-per-second rate, each point
// averaged over the trailing perfTPSWindow buckets.
func tpsSeries(samples []sim.PerfSample) []float64 {
	out := make([]float64, len(samples))
	ticks := 0
	for i, s := range samples {
		ticks += s.Ticks
		if i >= perfTPSWindow {
			ticks -= samples[i-perfTPSWindow].Ticks
		}
		span := time.Duration(min(i+1, perfTPSWindow)) * sim.PerfBucket
		out[i] = float64(ticks) / span.Seconds()
	}
	return out
}

// msSeries is each bucket's mean tick cost in milliseconds. A bucket with no
// ticks has no cost to report, so it is a gap (NaN) rather than a zero that
// would read as an impossibly fast tick.
func msSeries(samples []sim.PerfSample) []float64 {
	out := make([]float64, len(samples))
	for i, s := range samples {
		if s.Ticks == 0 {
			out[i] = math.NaN()
			continue
		}
		out[i] = durationMs(s.MeanTick())
	}
	return out
}

func durationMs(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func formatTPS(v float64) string {
	if v >= 100 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

func formatMs(v float64) string {
	switch {
	case v >= 100:
		return fmt.Sprintf("%.1fms", v)
	case v >= 10:
		return fmt.Sprintf("%.2fms", v)
	default:
		return fmt.Sprintf("%.3fms", v)
	}
}

// seriesSummary is the min/max/mean/last of a series' non-gap points.
type seriesSummary struct {
	last, lo, hi, mean float64
	valid              []float64 // the non-gap points, in order
}

func summarize(values []float64) (seriesSummary, bool) {
	var s seriesSummary
	for _, v := range values {
		if !math.IsNaN(v) {
			s.valid = append(s.valid, v)
		}
	}
	if len(s.valid) == 0 {
		return s, false
	}
	s.last, s.lo, s.hi = s.valid[len(s.valid)-1], s.valid[0], s.valid[0]
	sum := 0.0
	for _, v := range s.valid {
		s.lo, s.hi = min(s.lo, v), max(s.hi, v)
		sum += v
	}
	s.mean = sum / float64(len(s.valid))
	return s, true
}

// percentile returns the p-th percentile (0–100) of values by nearest rank.
func percentile(values []float64, p float64) float64 {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	i := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	return sorted[clamp(i, 0, len(sorted)-1)]
}

func tpsStats(values []float64, target int) []string {
	s, ok := summarize(values)
	if !ok {
		return []string{fmt.Sprintf("target %d", target)}
	}
	return []string{
		"last " + formatTPS(s.last),
		"min " + formatTPS(s.lo),
		"max " + formatTPS(s.hi),
		"avg " + formatTPS(s.mean),
		fmt.Sprintf("target %d", target),
	}
}

// msStats reports the tick cost over the samples on screen. "avg" is total
// work over total ticks, not the mean of the bucket means, so a quiet bucket
// with one tick does not weigh as much as a busy one with fifty; "worst" is
// the slowest single tick, which a bucket mean would smooth away; "budget" is
// the time one tick may take at the target rate before the sim falls behind.
func msStats(shown []sim.PerfSample, values []float64, target int) []string {
	budget := "budget " + formatMs(1000/float64(max(target, 1)))
	s, ok := summarize(values)
	if !ok {
		return []string{budget}
	}
	var busy, step, worst time.Duration
	ticks := 0
	for _, x := range shown {
		busy += x.Busy()
		step += x.Step
		worst = max(worst, x.MaxTick)
		ticks += x.Ticks
	}
	out := []string{"last " + formatMs(s.last)}
	if ticks > 0 {
		out = append(out, "avg "+formatMs(durationMs(busy)/float64(ticks)))
	}
	// Ordered by how much each tells you, since a narrow terminal cuts the
	// line from the right.
	out = append(out,
		"p95 "+formatMs(percentile(s.valid, 95)),
		"worst "+formatMs(durationMs(worst)),
		budget,
		"min "+formatMs(s.lo),
	)
	if busy > 0 {
		out = append(out, fmt.Sprintf("step %d%%", int(math.Round(100*float64(step)/float64(busy)))))
	}
	return out
}

// perfChart is one bordered chart panel: a title and stats line over a
// braille line graph with a labelled y axis and a time axis.
type perfChart struct {
	title  string
	style  lipgloss.Style
	stats  []string
	values []float64 // one per dot column, oldest first; NaN is a gap
	start  time.Time // wall time of values[0]
	span   time.Duration
	unit   string // appended to y-axis labels
}

func (c perfChart) render(width, height int) string {
	inner := panelInner(width)
	content := max(0, height-borderCells)
	lines := []string{cells.Truncate(
		labelStyle.Render(c.title)+"  "+c.style.Render(strings.Join(c.stats, "  ")), inner)}

	plotW := inner - perfLabelWidth - 1
	plotRows := content - 3 // title, axis rule, time labels
	switch {
	case len(c.values) == 0:
		lines = append(lines, "", helpStyle.Render(cells.Truncate("collecting samples…", inner)))
	case plotRows >= 1 && plotW >= 1:
		lines = append(lines, c.plot(plotW, plotRows)...)
		lines = append(lines,
			strings.Repeat(" ", perfLabelWidth)+"└"+strings.Repeat("─", plotW),
			timeAxis(c.start, c.span, perfLabelWidth+1, plotW))
	}
	return sidebarStyle.Width(width - borderCells).Height(content).MaxHeight(height).
		Render(strings.Join(lines, "\n"))
}

// plot draws the graph area: rows lines, each a y-axis label, the axis, and
// plotW braille cells.
func (c perfChart) plot(plotW, rows int) []string {
	s, _ := summarize(c.values)
	lo, hi := yRange(s)
	grid := newBraille(plotW, rows)
	dotRows := rows * 4

	prev := -1
	for x, v := range c.values {
		if math.IsNaN(v) {
			prev = -1
			continue
		}
		y := int(math.Round((hi - v) / (hi - lo) * float64(dotRows-1)))
		y = clamp(y, 0, dotRows-1)
		from := y
		if prev >= 0 {
			from = prev
		}
		// Join each point to the last with a vertical run in its own
		// column, the way gping draws a line one dot column at a time.
		for yy := min(from, y); yy <= max(from, y); yy++ {
			grid.set(x, yy)
		}
		prev = y
	}

	out := make([]string, rows)
	every := max(1, rows/6) // a label roughly every sixth of the height
	perRow := (hi - lo) / float64(max(rows-1, 1))
	for r := 0; r < rows; r++ {
		label, axis := "", "│"
		if r == 0 || r == rows-1 || (r%every == 0 && rows-1-r >= every/2+1) {
			label, axis = axisLabel(hi-float64(r)*perRow, perRow*float64(every), c.unit), "┤"
		}
		out[r] = fmt.Sprintf("%*s", perfLabelWidth, cells.Truncate(label, perfLabelWidth)) +
			axis + c.style.Render(grid.row(r))
	}
	return out
}

// axisLabel formats a y-axis value with just enough decimals that labels one
// step apart still read as different numbers: a rate holding at 400
// ±2 needs "399.5", not five rows of "400".
func axisLabel(v, step float64, unit string) string {
	decimals := 0
	if step > 0 {
		decimals = clamp(int(math.Ceil(-math.Log10(step))), 0, 3)
	}
	return fmt.Sprintf("%.*f%s", decimals, v, unit)
}

// yRange pads the data's range a little so the line does not ride the frame,
// and gives a flat line some height to sit in the middle of.
func yRange(s seriesSummary) (lo, hi float64) {
	lo, hi = s.lo, s.hi
	pad := (hi - lo) * 0.05
	if pad == 0 {
		pad = max(math.Abs(hi)*0.1, 1)
	}
	lo, hi = lo-pad, hi+pad
	if s.lo >= 0 {
		lo = max(lo, 0)
	}
	return lo, hi
}

// timeAxis places start, middle and end times under a plot that begins at
// column offset and is plotW cells wide, dropping any label that would
// collide with its neighbour.
func timeAxis(start time.Time, span time.Duration, offset, plotW int) string {
	const layout = "15:04:05"
	width := offset + plotW
	buf := []rune(strings.Repeat(" ", width))
	next := 0
	put := func(col int, t time.Time) {
		label := []rune(t.Format(layout))
		col = clamp(col, 0, width-len(label))
		if col < next || col < 0 {
			return
		}
		copy(buf[col:], label)
		next = col + len(label) + 1
	}
	put(offset, start)
	put(offset+plotW/2-len(layout)/2, start.Add(span/2))
	put(width-len(layout), start.Add(span))
	return strings.TrimRight(string(buf), " ")
}

// braille is a dot canvas drawn with Unicode braille cells, each two dots
// wide and four tall — eight times the resolution of plain characters, and
// what gives gping its fine line.
type braille struct {
	w, h  int
	cells []uint8
}

func newBraille(w, h int) braille { return braille{w, h, make([]uint8, w*h)} }

// brailleBits maps a dot's (column, row) within a cell to its bit in the
// U+2800 block. The numbering is historical — dots 1–3 run down the left
// column, 4–6 down the right, and 7–8 were added underneath later — hence
// the irregular table.
var brailleBits = [2][4]uint8{
	{0x01, 0x02, 0x04, 0x40},
	{0x08, 0x10, 0x20, 0x80},
}

func (b braille) set(x, y int) {
	if x < 0 || y < 0 || x >= b.w*2 || y >= b.h*4 {
		return
	}
	b.cells[(y/4)*b.w+x/2] |= brailleBits[x%2][y%4]
}

// row renders one line of cells. Empty cells are spaces rather than the blank
// braille pattern, which some fonts draw as faint dots.
func (b braille) row(r int) string {
	var sb strings.Builder
	for _, c := range b.cells[r*b.w : (r+1)*b.w] {
		if c == 0 {
			sb.WriteByte(' ')
		} else {
			sb.WriteRune(rune(0x2800) + rune(c))
		}
	}
	return sb.String()
}
