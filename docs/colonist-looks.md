# Colonist looks

> Part of the [mars-sim documentation](./README.md).

## What it is

In the browser, a colonist's figure on the map, in the roster and in the
inspector is drawn in their own skin tone and hair — 👨🏿‍🦰, 👱🏽‍♀️, 👴🏼 —
instead of the plain 👨 / 👩 / 🧑 the terminal draws. Each look is a list of
candidate emoji, best first; the page draws the first one its emoji font
renders as a single glyph. The TUI is unchanged.

## Source

- [`internal/glyphs/looks.go`](../internal/glyphs/looks.go) — `Look`, the
  `Looks` table, and `ForColonistLook` / `ForEntityLook` / `LookIndex`.
- [`internal/glyphs/looks_test.go`](../internal/glyphs/looks_test.go) — the
  candidate lists, and which entities get a look.
- [`internal/wire/wire.go`](../internal/wire/wire.go) — `HelloGlyphs.Looks`
  and `entityGlyph`, the index a frame carries.
- [`internal/wire/roster.go`](../internal/wire/roster.go),
  [`internal/wire/inspect.go`](../internal/wire/inspect.go) — the `look`
  field on roster rows, the inspected entity and a tile's creatures.
- [`web/src/emoji.ts`](../web/src/emoji.ts) — `pickGlyph`, the measuring
  picker.
- [`web/src/main.ts`](../web/src/main.ts) — `withLooks`, which resolves every
  look once per game and appends it to `hello.glyphs.symbols`.

## How it works

**The candidates (Go).** A look depends on the plain figure `ForColonist`
picks (gender × adult/senior), the skin tone, and the hair. For an adult the
list is: tone and hair together, tone alone, hair alone, the plain figure —
deduplicated, so a black- or brown-haired colonist (the figure's default hair)
gets just tone, then plain.

| Hair | Adult man / woman / person | Senior |
| --- | --- | --- |
| black, brown | 👨🏿 (tone only) | 👴🏿 |
| red, white, bald | 👨🏿‍🦰 · 👨🏿‍🦳 · 👨🏿‍🦲 (ZWJ hair component) | 👴🏿 |
| blonde | 👱🏿‍♂️ · 👱🏿‍♀️ · 👱🏿 (blond is its own base, not a component) | 👴🏿 |

Seniors (👴 👵 🧓) only take a tone: Unicode has no hair components for them.
A mutant (🧟) and a colonist drawn by their state (😱 💬 🔫 📦 …) have no look.

`glyphs.Looks` enumerates every look once (90 of them) in a fixed order. The
frame's `uint16` glyph index already had room: an index `len(All)+i` means
`Looks[i]` (`entityGlyph`). Hello sends `glyphs.looks`, and roster rows and
inspector payloads carry the colonist's `look` list beside the plain `glyph`.

**The pick (browser).** `pickGlyph` measures each candidate with a canvas 2D
context in the map's emoji font and takes the first that is no wider than 1.4×
the plain figure. Results are cached per list. `withLooks` runs that over
`hello.glyphs.looks` on arrival and appends the results to
`hello.glyphs.symbols`, so from then on every index a frame carries is a plain
index into `symbols` — the atlas, the hover line and the top bar never learn
that looks exist.

## Why it is this way

- **Why measure?** A font that does not know a sequence draws its parts side
  by side: 👨🏿‍🦰 becomes 👨🏿 + 🦰, and an unknown skin modifier becomes a
  colored square beside the figure. Which sequences fuse depends on the font
  and its version, so no table in Go can know. Measured in Chromium with Noto
  Color Emoji, a fused sequence is exactly the plain figure's width and an
  unfused one twice it, so the 1.4 threshold has wide margins either way.
  `Intl.Segmenter` does *not* answer this: it reports grapheme clusters per
  Unicode, not what the font draws.
- **Why the candidates in Go, not built in the page?** The figure is
  `ForColonist`'s choice (it depends on gender, age and traits), and the
  page would have to repeat it. Go already owns "what does this colonist look
  like"; the page only owns "what can this font draw".
- **Why not in the TUI?** Terminals are the reason `glyphs.All` is single code
  points: they disagree about the *width* of ZWJ and skin-tone sequences, and a
  wrong width shears the whole grid. That has been tried and reverted before;
  see [terminal-cell-widths.md](./terminal-cell-widths.md). Looks live outside
  `All` so the TUI's registry and its tests never see them.
- **Why append to `symbols` instead of a second table?** Every consumer
  already indexes `symbols`. Resolving once into the same table keeps looks to
  one function in the page.
- **Why tone before hair in the fallback?** When a font fuses only one, skin
  tone is the attribute the colonist's flavor text leads with, and the one
  Unicode has supported longer.

## Extending it

- A new hair or figure: add the case in `lookKey.build` (and `foldHair` if it
  is a new hair color) and its figure to the `figures` list in `init`. Append
  to orders, never reorder: nothing persists looks indexes across games, but
  the table should stay deterministic.
- Invariant (tested): every look ends in a symbol from `All`, so the plain
  figure is always the last resort, and `len(All)+len(Looks)` fits a `uint16`.
- Another surface that shows a colonist's figure as text: send the `look`
  list beside the `glyph` and render `pickGlyph(look, glyph)`.

## Related

- [terminal-cell-widths.md](./terminal-cell-widths.md) — why the TUI bans these sequences.
- [frontend-web.md](./frontend-web.md) — the emoji atlas looks are drawn into.
- [wire-format.md](./wire-format.md) — Hello's glyphs and the frame's glyph index.
- [heredity.md](./heredity.md) — where skin tone and hair come from.
