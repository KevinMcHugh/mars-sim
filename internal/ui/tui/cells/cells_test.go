package cells

import (
	"strings"
	"testing"
)

// The measurements the layout depends on: emoji are two cells, ASCII is one,
// and ANSI styling is free. These are the numbers len() and len([]rune())
// get wrong, which is why every caller goes through Width.
func TestWidthCountsCellsNotBytesOrRunes(t *testing.T) {
	cases := []struct {
		name string
		s    string
		want int
	}{
		{"ascii", "hello", 5},
		{"empty", "", 0},
		{"emoji", "\U0001F477", 2},
		{"emoji run", "\U0001F477\U0001F47D", 4},
		{"mixed", "a\U0001F477b", 4},
		{"ellipsis", Ellipsis, 1},
		{"box drawing", "█·", 2},
		{"styled text measures as its text", "\x1b[1;31mred\x1b[0m", 3},
		{"widest line wins", "ab\n\U0001F477\U0001F477\nc", 4},
	}
	for _, tc := range cases {
		if got := Width(tc.s); got != tc.want {
			t.Errorf("%s: Width(%q) = %d, want %d", tc.name, tc.s, got, tc.want)
		}
	}
}

// Fit is the invariant the whole renderer leans on: whatever goes in, exactly
// n cells come out. Nothing below a Fit-ed line can be shifted by it.
func TestFitAlwaysReturnsExactlyNCells(t *testing.T) {
	inputs := []string{
		"", "a", "hello world", strings.Repeat("x", 100),
		"\U0001F477", "\U0001F477\U0001F477\U0001F477abc", "ab\U0001F47Dcd",
		"\U0001F477 colonist   \U0001F47D alien",
		"\x1b[1mstyled \U0001F408 text\x1b[0m",
	}
	for _, in := range inputs {
		for n := 0; n <= 24; n++ {
			got := Fit(in, n)
			if w := Width(got); w != n {
				t.Errorf("Fit(%q, %d) = %q (%d cells), want exactly %d", in, n, got, w, n)
			}
		}
	}
}

// Truncate must never overshoot, even when the cluster at the boundary is two
// cells wide and only one cell remains — the case that splits a wide glyph in
// half and desyncs the line.
func TestTruncateNeverExceedsBudget(t *testing.T) {
	s := "\U0001F477\U0001F47D\U0001F408\U0001F401"
	for n := 1; n <= 10; n++ {
		if w := Width(Truncate(s, n)); w > n {
			t.Errorf("Truncate(%q, %d) is %d cells, over budget", s, n, w)
		}
	}
}

// Pad leaves an over-long string alone; only Fit is allowed to shorten.
func TestPadDoesNotShorten(t *testing.T) {
	if got := Pad("hello", 3); got != "hello" {
		t.Errorf("Pad(%q, 3) = %q, want it unchanged", "hello", got)
	}
	if got := Pad("ab", 5); got != "ab   " {
		t.Errorf("Pad(%q, 5) = %q, want %q", "ab", got, "ab   ")
	}
}
