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

`cells.Fit(s, n)` returns *exactly* n cells, always. Map rows, legend rows and
panel lines all go through it, which is what keeps a width surprise local to the
line it happens on instead of shearing the frame.

**A registry instead of constants, in two tiers.** Rendering goes through
`fitGlyph`, so nothing reaches the terminal unvetted.

An **atomic** glyph is a single code point with Emoji_Presentation=Yes. Terminals
agree on these, so the declared width is trustworthy without asking, and the
structural rules (no VS16, no ZWJ, no skin-tone modifier) are what keep it that
way. It carries a two-cell ASCII fallback:

```go
glyphBed: {symbol: glyphBed, cells: 2, fallback: "=="},
```

A **composed** glyph is a base figure plus a skin tone modifier and optionally a
ZWJ hair component — `👨🏿‍🦰`. These are never trusted. They must be probed, and
instead of an ASCII fallback they declare a `reduce`: the next rung down.

The three VS16 glyphs were replaced with single-code-point equivalents — `🥫`
`🛌` `💬`.

### The ladder

This is what makes composed glyphs safe to use at all. Each rung the terminal
refuses drops to a simpler sequence that still means the same colonist:

```
👨🏿‍🦰   →   👨🏿   →   👨   →   "M "
skin+hair   skin      plain   ASCII
```

`resolveGlyph` walks down until it finds a rung the probe accepted. A terminal
that fuses skin tones but not hair shows `👩🏿` instead of `👩🏿‍🦰` — and every
other glyph on screen stays emoji.

The response to a mismatch depends on the tier, and the split is the point:

| What failed | Response |
| --- | --- |
| An **atomic** glyph | The whole UI goes ASCII. Every table agrees on these, so a terminal that disagrees cannot be trusted with any of them. |
| A **composed** glyph | That glyph alone drops a rung. It only means this terminal will not fuse that sequence. |

Without the tier split, one unfusable hair sequence would take the entire map
down to ASCII with it.

**Asking the terminal.** No static table settles this, because the answer is a
property of the terminal, not of Unicode. So `VerifyGlyphWidths` measures for
real, before Bubble Tea takes the terminal: park the cursor with `\r`, print the
glyph, send `ESC[6n`, and read back `ESC[<row>;<col>R`. The column the cursor
landed in *is* the width that terminal paints, by definition. Any mismatch
switches the whole UI to the ASCII set and says so in the footer.

The probe sends an empty-string query first as a gate. A terminal that does not
speak CPR fails there, after 250 ms, before the probe has drawn anything into
the user's scrollback — and before it could leave ninety-odd unanswered queries
in flight.

Composed glyphs are combinatorial: six figures × five skin tones × four hair
states is 75 of them, 95 glyphs in total. One query-and-wait each is
imperceptible locally and several seconds over a slow SSH link, so queries are
**batched**: terminals process their input in order and queue one reply per
query, so a whole batch goes out in a single write and the replies come back in
the same order. 95 glyphs cost three round trips instead of ninety-five. Every
symbol is measured from column 1, so one unexpectedly wide glyph cannot skew the
next reading.

`-glyphs` controls this: `auto` (default, probe), `emoji` (skip the probe and
trust the table), `ascii` (force the fallback).

### Which check catches what

| Failure | Caught by |
| --- | --- |
| An **atomic** glyph uses VS16, a ZWJ, or a skin-tone modifier | `TestAtomicGlyphsAreUnambiguous` (structurally, and via cross-library disagreement) |
| A raw emoji literal bypasses the registry | `TestNoRawEmojiOutsideTheRegistry` (parses the package's own source) |
| A glyph renders at the wrong width, at any rung | `TestFitGlyphAlwaysFillsOneTile` |
| A composed glyph with no ladder, or a ladder that cycles or dead-ends | `TestComposedGlyphsDeclareALadder`, `TestLaddersTerminate` |
| The ladder stepping to the wrong rung | `TestLadderStepsDownOneRungAtATime` |
| One unfusable glyph downgrading everything | `TestOneUnfusableGlyphDoesNotDowngradeTheRest` |
| A colonist profile producing an unregistered glyph | `TestEveryColonistProfileMapsToARegisteredGlyph` |
| A reply attributed to the wrong glyph in a batch | `TestMeasureBatchPairsRepliesWithSymbols` |
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

**Why ban VS16 but allow ZWJ?** Because VS16 buys nothing and ZWJ buys
something. `🛏️` and `🛌` are the same picture; one is contested and the other is
not, so there is no reason to take the risk. A skin tone and hair colour, by
contrast, are information the simulation actually models — and they are only
available as composed sequences. So VS16 stays banned outright, and ZWJ is
allowed on the condition that it is measured and has somewhere to fall back to.

**Why does the ban not catch ZWJ sequences?** It cannot. The VS16 bug was
catchable statically because the width tables disagreed — which is what the
cross-library check exploits. For `👨🏿‍🦰` all three tables say two cells,
because that is the spec answer. There is no static signal at all, so static
vetting is structurally incapable of catching a terminal that will not fuse.
Only the probe can. This is the reason the probe had to exist before composed
glyphs could be adopted.

**Why does an atomic failure still take everything down?** A half-emoji,
half-ASCII map is harder to read than either, and a terminal that gets a single
code point's width wrong has not earned trust about the rest. The ladder is the
right tool for "this terminal cannot fuse a four-code-point sequence", not for
"this terminal cannot measure `🟫`".

**Why is `clampFrame` separate from the renderers?** It truncates any line that
would exceed the terminal — a net under the whole frame. But a net that silently
catches things hides the bug it caught, which is roughly how this survived last
time. So `render()` is `clampFrame(renderFrame())`, and
`TestFrameNeverExceedsTerminalWidth` asserts that `clampFrame` is a *no-op*: the
renderers must be right, and the net is there for the case we did not think of.

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

**Adding an atomic glyph** is a two-line edit to `glyphs.go`: a `glyphX`
constant and a `glyphRegistry` entry with its declared width and an ASCII
fallback. The tests enforce the rest — if the glyph carries a variation
selector, a ZWJ or a skin tone modifier, or if the width tables disagree about
it, `go test` fails and names the reason. Pick a single code point with
Emoji_Presentation=Yes and it will pass.

**Adding a composed glyph** means extending `buildGlyphRegistry` so the new
sequence declares a `reduce` pointing at an existing, simpler rung. The
reduction must drop a suffix rather than change the figure — dropping detail,
not swapping the colonist for a different one — and the ladder must reach an
atomic glyph. Both are tested. The probe then covers it automatically.

Invariants a change here must preserve:

- Nothing measures a display string with `len()`, `len([]rune(...))`, or `%-Ns`.
  Use `cells`.
- Every glyph reaches the terminal through `fitGlyph`.
- `colonistGlyph` returns the *most specific* sequence; reducing it to something
  the terminal can paint is `fitGlyph`'s job, so callers never branch on
  terminal support.
- A panel width constant is the panel's *total* footprint including its border;
  `lipgloss`'s `Style.Width` sets the content box, so renderers pass
  `Width(total - borderCells)`.
- `clampFrame` stays a no-op in the tests.

## Related

- [frontend-tui.md](./frontend-tui.md) — the screens these glyphs are drawn on.
- [cli.md](./cli.md) — the `-glyphs` flag.
