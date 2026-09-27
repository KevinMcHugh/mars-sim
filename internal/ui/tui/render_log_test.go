package tui

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// letters keeps only letters and digits, lowercased, so a wrapped, bordered,
// styled frame can be checked for the words of an event without caring where
// the line breaks fell.
func letters(s string) string {
	var b strings.Builder
	esc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			esc = true
		case esc:
			if unicode.IsLetter(r) {
				esc = false
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// withANSI256 forces 256-color output for the test. View renders through
// lipgloss's default profile, which is colorless when stdout is not a
// terminal — the real TUI is one, and the stripes are what this checks.
func withANSI256(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

func TestLogKindLabelsFitTheColumn(t *testing.T) {
	for k := sim.LogNote; k < sim.LogKindCount; k++ {
		if w := cells.Width(k.String()); w > logKindColumn {
			t.Errorf("%q is %d cells, wider than the %d-cell type column", k, w, logKindColumn)
		}
	}
}

func TestLogColumnHeaderLinesUpWithTheColumns(t *testing.T) {
	const inner = 60
	styled := logColumnHeader(inner)
	if !strings.Contains(styled, "type") || !strings.Contains(styled, "event") {
		t.Fatalf("column header = %q", styled)
	}
	kindW, _, show := logColumns(inner)
	if !show {
		t.Fatal("expected the type column at this width")
	}
	header := cells.Pad("type", kindW) + strings.Repeat(" ", logKindGap) + "event"
	rows := logVisualRows([]sim.LogEntry{{Kind: sim.LogDeath, Text: "Ada Fields starved to death."}}, inner)
	if strings.Index(header, "event") != strings.Index(rows[0].text, "Ada Fields") {
		t.Fatalf("header and sentence columns differ:\n%q\n%q", header, rows[0].text)
	}
}

func TestLogKindColumnAlignsTheSentence(t *testing.T) {
	entries := []sim.LogEntry{
		{Kind: sim.LogDeath, Text: "Ada Fields starved to death."},
		{Kind: sim.LogBuildComplete, Text: "A dormitory with a very long name is complete and the crew moves the bunks in before night."},
	}
	rows := logVisualRows(entries, 60)
	if len(rows) < 3 {
		t.Fatalf("expected the build line to wrap, got %#v", rows)
	}
	deathAt := strings.Index(rows[0].text, "Ada Fields")
	buildAt := strings.Index(rows[1].text, "A dormitory")
	if deathAt < 0 || deathAt != buildAt {
		t.Fatalf("sentences do not share a column: death at %d (%q), build at %d (%q)", deathAt, rows[0].text, buildAt, rows[1].text)
	}
	if !strings.HasPrefix(rows[0].text, "death") || !strings.HasPrefix(rows[1].text, "build complete") {
		t.Fatalf("type column missing:\n%s\n%s", rows[0].text, rows[1].text)
	}
	if strings.Contains(rows[2].text, "build") {
		t.Fatalf("wrapped line repeated the type: %q", rows[2].text)
	}
	if !strings.HasPrefix(rows[2].text, strings.Repeat(" ", logKindColumn+logKindGap)) {
		t.Fatalf("wrapped line is not indented under the sentence: %q", rows[2].text)
	}
}

func TestWrapLogEntryKeepsTheWords(t *testing.T) {
	const text = "alpha bravo charlie delta"
	got := wrapLogEntry(text, 12)
	if strings.Join(got, " ") != text {
		t.Fatalf("wrap dropped or cut words: %q", got)
	}
	for _, line := range got {
		if cells.Width(line) > 12 {
			t.Errorf("line %q is %d cells, wider than 12", line, cells.Width(line))
		}
		if strings.Contains(line, cells.Ellipsis) {
			t.Errorf("wrapped line is marked truncated: %q", line)
		}
	}
	if len(got) < 2 {
		t.Fatalf("expected the sentence to wrap, got %q", got)
	}
}

func TestWrapLogEntrySplitsAWordWiderThanThePanel(t *testing.T) {
	got := wrapLogEntry("abcdefghij", 4)
	if strings.Join(got, "") != "abcdefghij" {
		t.Fatalf("hard cut dropped characters: %q", got)
	}
	for _, line := range got {
		if cells.Width(line) > 4 {
			t.Errorf("line %q is %d cells, wider than 4", line, cells.Width(line))
		}
	}
}

func logEntry(text string) sim.LogEntry { return sim.LogEntry{Text: text} }

func TestLogEntriesDroppedCountsTheRingSlide(t *testing.T) {
	prev := []sim.LogEntry{logEntry("a"), logEntry("b"), logEntry("c")}
	if got := logEntriesDropped(prev, []sim.LogEntry{logEntry("a"), logEntry("b"), logEntry("c"), logEntry("d")}); got != 0 {
		t.Errorf("append dropped %d, want 0", got)
	}
	if got := logEntriesDropped(prev, []sim.LogEntry{logEntry("b"), logEntry("c"), logEntry("d")}); got != 1 {
		t.Errorf("one slide dropped %d, want 1", got)
	}
	if got := logEntriesDropped(prev, []sim.LogEntry{logEntry("z")}); got != len(prev) {
		t.Errorf("no overlap dropped %d, want %d", got, len(prev))
	}
}

func logLines(n int) []sim.LogEntry {
	lines := make([]sim.LogEntry, n)
	for i := range lines {
		lines[i] = logEntry(fmt.Sprintf("event-%02d happened", i))
	}
	return lines
}

// toLog opens the log tab. The tab count is asserted via modeLog so a
// rotation change fails here instead of as a missing string.
func toLog(t *testing.T, snap *sim.Snapshot, w, h int) tea.Model {
	t.Helper()
	var model tea.Model = New(nil, nil)
	model, _ = model.Update(tea.WindowSizeMsg{Width: w, Height: h})
	model, _ = model.Update(snapshotMsg{snap: snap})
	for model.(Model).mode != modeLog {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
		if model.(Model).mode == modeMap {
			t.Fatal("tab rotation never reached the log screen")
		}
	}
	return model
}

func TestLogTabShowsTheFullEntryWrapped(t *testing.T) {
	const event = "Amara Moreau has mutated, grew a tail and a second pair of arms, and now stands a head taller than the rest of the crew"
	snap := makeSnapshot()
	snap.Log = []sim.LogEntry{logEntry(event), logEntry("A bunk is bolted into the dormitory floor.")}

	model := toLog(t, snap, 120, 24)
	out := model.View()
	if !strings.Contains(letters(out), letters(event)) {
		t.Fatalf("log tab did not show the whole event:\n%s", out)
	}
	if strings.Contains(out, cells.Ellipsis) {
		t.Fatalf("log tab truncated a line:\n%s", out)
	}
	if !strings.Contains(out, "COLONY LOG") || !strings.Contains(out, "live") {
		t.Fatalf("log tab missing its title:\n%s", out)
	}
}

func TestLogTabFollowsTheTailUntilScrolled(t *testing.T) {
	snap := makeSnapshot()
	snap.Log = logLines(40)
	model := toLog(t, snap, 80, 16)

	out := model.View()
	if !strings.Contains(out, "event-39 happened") {
		t.Fatalf("live view hid the newest event:\n%s", out)
	}
	if strings.Contains(out, "event-00 happened") {
		t.Fatalf("live view showed the oldest event:\n%s", out)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	out = model.View()
	if strings.Contains(out, "event-39 happened") {
		t.Fatalf("up did not leave the live tail:\n%s", out)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	out = model.View()
	if !strings.Contains(out, "event-39 happened") || !strings.Contains(out, "live") {
		t.Fatalf("down from one line up did not resume the live feed:\n%s", out)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	out = model.View()
	if !strings.Contains(out, "event-00 happened") || strings.Contains(out, "event-39 happened") {
		t.Fatalf("home did not park at the start of the log:\n%s", out)
	}

	// Park on a middle entry, then let the ring drop the lines above it.
	// The snapshot the model already holds must not be the one we mutate:
	// Update compares the previous log with the new one, and they have to
	// be different slices for that drop to be visible.
	parked := model.(Model)
	parked.logScrolled = true
	parked.logAnchor = 10
	parked.logAnchorRow = 0
	model = parked
	next := *snap
	next.Log = append(append([]sim.LogEntry{}, snap.Log[4:]...),
		logEntry("event-40 happened"), logEntry("event-41 happened"),
		logEntry("event-42 happened"), logEntry("event-43 happened"))
	model, _ = model.Update(snapshotMsg{snap: &next})
	out = model.View()
	if !strings.Contains(out, "event-10 happened") {
		t.Fatalf("scrollback lost the entry it was parked on:\n%s", out)
	}
	if strings.Contains(out, "event-43 happened") {
		t.Fatalf("a new event yanked the scrolled-back view:\n%s", out)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	out = model.View()
	if !strings.Contains(out, "event-43 happened") || !strings.Contains(out, "live") {
		t.Fatalf("end did not return to the live tail:\n%s", out)
	}
}

func TestLogStripesStayWithTheEntry(t *testing.T) {
	withANSI256(t)
	snap := makeSnapshot()
	snap.Log = []sim.LogEntry{
		logEntry("alpha bravo charlie delta echo foxtrot golf hotel india juliet"),
		logEntry("the second event is short"),
	}
	model := toLog(t, snap, 40, 24)
	out := model.View()

	var alpha, second string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "alpha"):
			alpha = line
		case strings.Contains(line, "second event"):
			second = line
		}
	}
	if alpha == "" || second == "" {
		t.Fatalf("expected both entries on screen:\n%s", out)
	}
	if !strings.Contains(alpha, "38;5;252") {
		t.Fatalf("first entry did not use the even stripe:\n%s", alpha)
	}
	if strings.Contains(alpha, "48;5;236") {
		t.Fatalf("first entry used the odd stripe:\n%s", alpha)
	}
	if !strings.Contains(second, "48;5;236") {
		t.Fatalf("second entry did not use the odd stripe:\n%s", second)
	}

	// A wrapped entry is one stripe, so the continuation line matches the
	// line the sentence started on.
	var cont string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "juliet") {
			cont = line
		}
	}
	if cont == "" {
		t.Fatalf("the long entry was not wrapped:\n%s", out)
	}
	if !strings.Contains(cont, "38;5;252") || strings.Contains(cont, "48;5;236") {
		t.Fatalf("continuation line changed stripe:\n%s", cont)
	}

	// Drop the first entry. "the second event" was the odd stripe and must
	// stay that way even though it is now index 0. The new frame is a
	// different snapshot: Update diffs it against the one already held.
	next := *snap
	next.Log = []sim.LogEntry{logEntry("the second event is short"), logEntry("a third event arrives")}
	model, _ = model.Update(snapshotMsg{snap: &next})
	out = model.View()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "second event") {
			if !strings.Contains(line, "48;5;236") {
				t.Fatalf("stripe flipped when the ring slid:\n%s", line)
			}
			return
		}
	}
	t.Fatalf("second event missing after the slide:\n%s", out)
}

func TestSidebarWrapsLogLines(t *testing.T) {
	const event = "Amara Moreau has mutated, grew a tail and a second pair of arms, and now stands a head taller than the rest of the crew"
	m := New(nil, nil)
	m.termW, m.termH = 100, 40
	snap := busySnapshot()
	snap.Log = []sim.LogEntry{logEntry(event)}
	m.latest = snap

	out := m.renderMapSidebar()
	if !strings.Contains(letters(out), letters(event)) {
		t.Fatalf("sidebar truncated the log entry:\n%s", out)
	}
	if strings.Contains(out, cells.Ellipsis) {
		t.Fatalf("sidebar log still uses an ellipsis:\n%s", out)
	}
}

func TestLogTabEscReturnsToMap(t *testing.T) {
	model := toLog(t, makeSnapshot(), 80, 24)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m := model.(Model); m.mode != modeMap {
		t.Fatalf("mode after esc = %v, want modeMap", m.mode)
	}
}

func TestLogTabBeforeAnyEvent(t *testing.T) {
	snap := makeSnapshot()
	snap.Log = nil
	out := toLog(t, snap, 80, 24).View()
	if !strings.Contains(out, "Nothing logged yet.") {
		t.Fatalf("empty log did not say so:\n%s", out)
	}
}
