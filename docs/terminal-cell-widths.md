# Terminal cell widths

> Part of the [mars-sim documentation](./README.md).

## What it is

Everything that keeps the TUI's grid aligned: one function for measuring how
wide a string is in terminal cells, a registry of the glyphs we are willing to
draw, a startup probe that checks those widths against the actual terminal, and
an ASCII fallback set for when the terminal disagrees.

This exists because the same bug kept coming back. The map's brick wall would go
jagged, the sidebar's border would be clipped or come out short, and the
misalignment would persist for the rest of the session. Every previous fix
treated a symptom.

## Source

- `internal/ui/tui/cells/cells.go` — `Width`, `Truncate`, `Pad`, `Fit`. The only
  sanctioned way to measure or size a string.
- `internal/ui/tui/glyphs.go` — the glyph registry: declared widths, ASCII
  fallbacks, and `fitGlyph`.
- `internal/ui/tui/probe.go` — the cursor-position-report probe.
- `main.go` — the `-glyphs` flag and `setUpGlyphs`.
- `internal/ui/tui/glyphs_test.go` — registry invariants and the source scan.
- `internal/ui/tui/layout_test.go` — whole-frame rectangularity.
- `internal/ui/tui/probe_test.go` — CPR parsing and the give-up path.

## How it works

### The three things that were wrong

**1. Ambiguous glyphs.** Three glyphs were an emoji followed by U+FE0F, the
variation selector that asks for emoji presentation: `🍽️` (U+1F37D FE0F), `🛏️`
(U+1F6CF FE0F) and `🗣️` (U+1F5E3 FE0F). Whether that selector makes a glyph two
cells wide is a per-terminal decision. Our own dependencies disagree about it
today:

| glyph | `x/ansi` | `uniseg` | `go-runewidth` |
| --- | --- | --- | --- |
| `🛏️` U+1F6CF FE0F | 2 | 2 | **1** |
| `🛌` U+1F6CC | 2 | 2 | 2 |

Every map row containing a bunk was one cell narrower than the renderer believed.
That is the jagged wall.

**2. Nothing pinned a line to a width.** A one-cell error in a map row shifted
everything after it, which is how a map glitch became a clipped sidebar. It also
defeated Bubble Tea's repaint: the standard renderer emits an
erase-to-end-of-line only when it measures a line as *narrower* than the
terminal (`standard_renderer.go`), so a line it measured as full-width left the
previous frame's cells on screen underneath it.

**3. The wrong counters.** `truncate` counted runes and the gauge lines padded
with `%-7s` / `%-12s`, which counts bytes. An emoji is 1 rune, 4 bytes and 2
cells; all three numbers differ and only the last one matters.

### The fix

**One measurement function.** `cells.Width` is `ansi.StringWidth` — deliberately
the exact function Bubble Tea's renderer and `lipgloss.Width` call. Agreeing
with the renderer is not a style preference: if our arithmetic and the
renderer's disagree, the renderer makes the wrong call about erasing the line.

`cells.Fit(s, n)` returns *exactly* n cells, always. Legend rows and panel lines
go through it, which is what keeps a width surprise local to the line it happens
on instead of shearing the frame.

Map rows are exact by construction instead. Every tile is a `fitGlyph` result of
exactly `tileWidth` cells, and `TestAdjacentGlyphsNeverMerge` checks every
ordered pair of glyphs in both sets to prove no two fuse into one grapheme
cluster when drawn side by side, so a row's width is simply the sum of its
tiles. Map rows used to be passed through `cells.Fit` as well, but that cost a
grapheme scan of every row on every frame and could not catch anything:
`cells.Fit` uses the same width table `fitGlyph` already did. Disagreement with
the *terminal* is the startup probe's job, not the renderer's.

**A registry instead of constants.** Each glyph declares its width and carries a
two-cell ASCII fallback:

```go
glyphBed: {glyphBed, 2, "=="},
```

Rendering goes through `fitGlyph`, so nothing reaches the terminal unvetted. The
three VS16 glyphs were replaced with single-code-point equivalents — `🥫` `🛌`
`💬` — which are the one case terminals agree on: a single code point with
Emoji_Presentation=Yes is two cells everywhere.

**Asking the terminal.** No static table settles this, because the answer is a
property of the terminal, not of Unicode. So `VerifyGlyphWidths` measures for
real, before Bubble Tea takes the terminal: park the cursor with `\r`, print the
glyph, send `ESC[6n`, and read back `ESC[<row>;<col>R`. The column the cursor
landed in *is* the width that terminal paints, by definition. Any mismatch
switches the whole UI to the ASCII set and says so in the footer.

The probe sends an empty-string query first as a gate. A terminal that does not
speak CPR fails there, after 250 ms, before the probe has drawn anything into
the user's scrollback — and before it could leave eighteen more unanswered
queries in flight.

`-glyphs` controls this: `auto` (default, probe), `emoji` (skip the probe and
trust the table), `ascii` (force the fallback).

### Which check catches what

| Failure | Caught by |
| --- | --- |
| A new glyph uses VS16, a ZWJ, or a skin-tone modifier | `TestGlyphRegistryIsUnambiguous` (structurally, and via cross-library disagreement) |
| A raw emoji literal bypasses the registry | `TestNoRawEmojiOutsideTheRegistry` (parses the package's own source) |
| A glyph renders at the wrong width | `TestFitGlyphAlwaysFillsOneTile` |
| A row or panel is mis-sized | `TestMapRowsAreAllTheSameWidth`, `TestPanelsRenderAtTheirDeclaredWidth` |
| Any frame that would wrap | `TestFrameNeverExceedsTerminalWidth` |
| The terminal disagrees at runtime | the startup probe |

## Why it is this way

**Why not just a width table keyed by glyph?** That was the first instinct, and
it is half the answer: the registry *is* that table. But a table only records
what we believe. It cannot be right about a terminal we have never run on,
because terminals build their own width tables from different Unicode versions
and make their own calls about emoji presentation. A table plus a probe is
correct; a table alone is a better guess. The probe is what makes this "once and
for all" rather than "once more".

**Why ban VS16 instead of measuring it?** We could measure it and pick per
terminal. Banning is better: the sequences terminals agree on are a large and
perfectly readable set, so accepting a per-terminal split buys nothing. This is
the third time an exotic sequence has been reverted here — skin tone modifiers
and ZWJ hair components went the same way (see the commit history around
`97996ba` and `0649c55`). The ban is those reverts turned into a rule a test
enforces, instead of knowledge that has to survive in someone's head.

**Why all-or-nothing on the ASCII fallback?** A half-emoji, half-ASCII map is
harder to read than either. And a terminal that got one glyph's width wrong has
not earned trust about the rest.

**Why is `clampFrame` separate from the renderers?** It truncates any line that
would exceed the terminal — a net under the whole frame. But a net that silently
catches things hides the bug it caught, which is roughly how this survived last
time. So `render()` is `clampFrame(renderFrame())`, and
`TestFrameNeverExceedsTerminalWidth` asserts that `clampFrame` is a *no-op*: the
renderers must be right, and the net is there for the case we did not think of.

The net is kept cheap rather than removed. Measuring every line of every frame
was about a fifth of the render, so `clampFrame` reuses the widths of lines that
are unchanged since the previous frame (`renderCache` in `render_cache.go`).
Width is a pure function of the string, so a reused width is exactly as correct
as a fresh scan, and it is still compared against the *current* terminal width
— a line that fit before a resize is trimmed after it
(`TestClampFrameRechecksCachedLinesAfterResize`).

**Two layout bugs found while testing this.** Both the map/sidebar split and the
roster and job board's list/detail split floored one panel at a minimum width
and gave the neighbour whatever was left — which, in a terminal that had no room
for both, was a negative number floored back up to the minimum. The panels added
up to more than the terminal and the second one was pushed off the right edge.
Below the width where both fit, the second panel is now dropped instead
(`splitPanels`, `sidebarFits`). This was a *layout* bug, unrelated to emoji, that
produced the same visible symptom — worth knowing, because it means "the sidebar
looks wrong" does not always mean a glyph misbehaved.

**Known gap.** A missing glyph in the terminal's font is invisible to the probe:
the terminal advances the cursor by the expected number of cells and paints
tofu. That is what `-glyphs ascii` is for.

## Extending it

**Adding a glyph** is a two-line edit to `glyphs.go`: a `glyphX` constant and a
`glyphRegistry` entry with its declared width and an ASCII fallback. The tests
enforce the rest — if the glyph carries a variation selector, a ZWJ or a skin
tone modifier, or if the width tables disagree about it, `go test` fails and
names the reason. Pick a single code point with Emoji_Presentation=Yes and it
will pass.

Invariants a change here must preserve:

- Nothing measures a display string with `len()`, `len([]rune(...))`, or `%-Ns`.
  Use `cells`.
- Every glyph reaches the terminal through `fitGlyph`.
- No two glyphs fuse when drawn side by side (`TestAdjacentGlyphsNeverMerge`);
  map rows rely on it instead of being measured.
- A panel width constant is the panel's *total* footprint including its border;
  `lipgloss`'s `Style.Width` sets the content box, so renderers pass
  `Width(total - borderCells)`.
- `clampFrame` stays a no-op in the tests.

**A glyph chosen by data outside this package**, rather than always by code
in it, showed up for the first time with `AlienSpecies.Emoji` (see
[lore.md](./lore.md)): `sim` rolls a plain string from a YAML file, with no
idea this package's rules exist. The pattern that keeps the guarantees above
intact is `alienGlyph`: check the runtime string against `glyphRegistry`
(an exact key match — the string either *is* one of the vetted symbols
above, or it isn't) and fall back to a plain, always-registered glyph for
anything that doesn't match, rather than ever handing an unrecognized
string to `fitGlyph`'s registry lookup and trusting its unregistered-input
path (`cells.Fit`) to make it safe. That fallback path exists and is
genuinely grid-safe — it forces exactly `tileWidth` cells no matter what —
but it is not probed, has no real ASCII fallback, and its correctness
depends on `ansi.StringWidth` agreeing with the actual terminal, which is
exactly the class of disagreement this whole system exists to catch instead
of trust. Any future glyph sourced from outside this package should draw
from a registry-checked palette the same way, not lean on that fallback as
a feature.

## Related

- [frontend-tui.md](./frontend-tui.md) — the screens these glyphs are drawn on.
- [cli.md](./cli.md) — the `-glyphs` flag.
- [lore.md](./lore.md) — `alienGlyph`, the one place a runtime string from
  outside this package (a rolled species' `Emoji`, ultimately sourced from a
  YAML file) is checked against the registry before it can reach the map.
