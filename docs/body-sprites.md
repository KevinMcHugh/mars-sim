# Body sprites and map scale

> Part of the [mars-sim documentation](./README.md).

**S1 (scale) and S2 (mutant paper dolls) are built. S3 is a proposal.** When a phase
ships, move its text into the present tense and note anywhere the build
departed from the plan, as [z-levels.md](./z-levels.md) does.

## What it is

Two browser-map changes that share a payoff: you can see what a body is like.

1. **Scale (S1).** Every colonist and alien is drawn at a size that follows its
   height. A ten-foot mutant stands over the dormitory door, a two-foot one
   barely clears the table, a grub is a speck and a queen fills the tunnel.
   Before S1, every entity was exactly one tile.
2. **Paper dolls (S2, S3).** A colonist is drawn as a composited SVG figure:
   a base body in their own skin and hair, with layers for the mutant parts
   they actually grew (third arm, extra eye, tail, vestigial twin). S2 does
   this for mutants only, replacing 🧟. S3 extends it to every colonist.

The terminal is unchanged: one cell per tile, emoji only
([terminal-cell-widths.md](./terminal-cell-widths.md)). Nothing here touches
the simulation, so no seed changes.

## Source

S1, built:

- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go):
  `EntityView.HeightCM`, set in `entityView`. Tested by `TestEntityViewHeight`
  in [`alien_lifecycle_test.go`](../internal/sim/alien_lifecycle_test.go).
- [`internal/wire/encoder.go`](../internal/wire/encoder.go),
  [`internal/wire/wire.go`](../internal/wire/wire.go): the frame's
  `uint16 height × N` entities array, `entityHeight`, `Version` 7.
  [`web/wire/decode.js`](../web/wire/decode.js) reads it as
  `entities.height`.
- [`web/src/map/bodyscale.ts`](../web/src/map/bodyscale.ts): `bodyScale`, the
  curve from centimetres to tiles, and its constants. Tested by
  `bodyscale.test.mjs`.
- [`web/src/map/renderer.ts`](../web/src/map/renderer.ts): the per-instance
  `aBody` attribute and the `uFeet` uniform in `SPRITE_VS`, and the
  biggest-first sort in `update`.

S2, built:

- [`internal/glyphs/dolls.go`](../internal/glyphs/dolls.go): the key's
  digits (`DollFigures`, `DollTones`, `DollHairs`, `DollParts`), `NumDolls`,
  `DollKey` (the map's) and `RestingDollKey` (the roster's). Tested in
  [`dolls_test.go`](../internal/glyphs/dolls_test.go).
- [`internal/wire/wire.go`](../internal/wire/wire.go): `HelloDolls`, and the
  doll range in `entityGlyph`, which now takes the game's sprite count.
  `EntityTopic.Doll` ([`inspect.go`](../internal/wire/inspect.go)) and
  `RosterRow.Doll` ([`roster.go`](../internal/wire/roster.go)). Tested in
  [`dolls_test.go`](../internal/wire/dolls_test.go).
- [`web/src/map/dolls.ts`](../web/src/map/dolls.ts): `dollDigits` and
  `composeDoll`, the part library. Tested by `dolls.test.mjs`.
- [`web/src/map/dollpool.ts`](../web/src/map/dollpool.ts): `DollPool`, the
  atlas cells dolls are painted into. Tested by `dollpool.test.mjs`.
- [`web/src/map/atlas.ts`](../web/src/map/atlas.ts): `buildAtlas`'s
  `reserve` cells and `paintCell`. The renderer's `atlasGlyph` maps a frame's
  doll index to its cell, or to the fallback.
- [`web/src/ui/sprite.ts`](../web/src/ui/sprite.ts): `dollURL`, for the
  Inspect heading and the roster rows.

## How it works

### S1: scale

**What Go sends.** Go sends a height in centimetres, not a scale. Which height
a body has is a simulation fact. How big that looks on a tile is a display
choice, and the page owns it, the same way Go owns "which figure" and the page
owns "what the font can draw" in [colonist-looks.md](./colonist-looks.md).

| Entity | `HeightCM` |
| --- | --- |
| colonist | `Profile.HeightCM` (61 to 305 cm after mutation; see [mutation.md](./mutation.md)) |
| alien | `(HeightMinCM + HeightMaxCM) / 2 × form.SizePct / 100` |
| everything else | 0, meaning "draw at 1.0" |

Aliens have no per-specimen height today, only the species' range, so every
adult of a species is the same size. That is fine for S1. See "Extending it".

**What the page does with it.** Display size, in tiles, is

```
scale = clamp((heightCM / 178) ^ 0.75, 0.35, 2.0)
```

178 cm, an average colonist, stays exactly one tile, the size everything is
drawn at now. Some sample sizes:

| Body | cm | tiles |
| --- | --- | --- |
| two-foot mutant (floor) | 61 | 0.45 |
| average colonist | 178 | 1.00 |
| ten-foot mutant (ceiling) | 305 | 1.50 |
| egg at 10% of a 150 cm adult | 15 | 0.35 (clamped) |
| huge adult | 320 | 1.55 |
| queen at 200% of a huge adult | 640 | 2.0 (clamped) |

The exponent and the clamps are first guesses, checked by eye in a sandbox
world ([sandbox.md](./sandbox.md)). They are constants in `bodyscale.ts`, not
`sim.Config` keys, because they never affect the simulation. Bear in mind that
the committed species pack runs big: most adults stand 2 to 4 m, so most
packed aliens draw at 1.3 to 1.8 tiles and their queens at the 2.0 cap. If the
map feels crowded, lower `MAX_SCALE` or the exponent before anything else.

**Rendering.**

- The page turns each height into a scale (`bodyScale`) as it uploads the
  frame, and `SPRITE_VS` takes it as a per-instance float `aBody`, multiplied
  into `uSize` when `uFeet` is set. The sprite is then **anchored at the
  bottom centre** of its tile, not the middle; at scale 1 the two anchors
  give the same quad, so an unscaled body draws exactly as before. A giant grows up into the
  tile above, and a small body sits on its own floor instead of floating in
  the middle of the tile.
- **Draw order:** biggest first. A 2-tile queen must not hide the colonist
  standing in front of her. The page sorts the instance arrays by scale
  before upload. N is the number of entities in view, so a per-frame sort is
  cheap. Ties keep frame order, which is entity order.
- **Zoomed out** (dot mode, `uGlyphs` false), sizes stay as they are. Dots are
  sized in pixels so they stay visible, and scaling them would only make small
  creatures vanish.
- **Refuse** (corpses, gore) does not scale: its VAO has no `aBody` buffer
  and reads the attribute's generic value, 1, and its pass leaves `uFeet`
  off.
- **Hover and click** stay per tile. A queen spilling into the next tile is
  still picked from her own tile.
- **Atlas cells** stay 128 px. At the top zoom a 2-tile sprite is upscaled
  about 2×, which is soft but acceptable. Increase `CELL` only if it looks bad.
  It costs atlas memory for every symbol, not just the big ones.

**Inspector and lore.** These panels already print heights. S1 changes no
text. A side-by-side silhouette ("as tall as 1.7 colonists") is a possible
later addition.

### S2: paper dolls for mutants

**The key.** A doll is a pure function of four small enums, laid out
mixed-radix like `Looks`:

| Digit | Values | Count |
| --- | --- | --- |
| figure | man, woman, person × adult, senior | 6 |
| skin tone | `SkinLight` … `SkinDark` | 5 |
| hair | black, brown, blonde, red, white, bald | 6 |
| mutant parts | bitmask of the four mutant `BodyPart`s with `MaxParts > 0` | 16 |

That gives 6 × 5 × 6 × 16 = 2,880 keys. A frame's glyph index
`len(All) + len(Looks) + len(Sprites) + key` means doll `key`, which fits
easily in the `uint16`. No new frame field is needed, and no table has to
grow mid-game. A colonist who mutates just starts sending a different index.

`glyphs.ForColonist` still returns 🧟 for a mutant (the terminal and every
text surface keep it), and `DollKey` gives the map a doll in its place. Hello
sends `glyphs.dolls` (`HelloDolls`): `base`, `count`, the digit values in
order (`figures`, `tones`, `hairs`, `parts`) and `fallback`, the symbol to
draw until a doll is painted (🧟). The page unpacks a key with those radixes
(`dollDigits`), so the digit order is the one thing Go and the page must agree
on, and both tests pin it.

`base` depends on the game's sprite count, so `entityGlyph` takes it: the
encoder passes `len(snap.Sprites)`.

**Not in `symbols`.** Every symbol gets an atlas cell, and 2,880 cells × 128²
× 4 bytes is about 190 MB of texture. So dolls are **never** appended to
`symbols`, unlike looks and pack sprites. Instead:

1. The page decodes the frame. For each glyph index at or past the doll base,
   it looks the key up in `dollCells: Map<key, cell>`.
2. If a cell exists, the instance's glyph becomes that cell. If not, it
   becomes `dollFallback` for this frame, and `composeDoll(key)` is queued.
3. A composed doll is decoded through an `Image` (the same `loadSVG` path the
   pack sprites use, so no markup ever enters the DOM), drawn into a free cell
   from a fixed pool reserved at the end of the atlas (`DOLL_CELLS`, 64), in
   the atlas's canvas too (so the full re-upload when pack sprites settle
   keeps it), and uploaded with `texSubImage2D`. Mipmaps are then
   regenerated, and the map is marked dirty.
4. If the pool is full, the least recently drawn doll's cell is evicted, but
   only once it has gone undrawn for `STALE_FRAMES` (30) frames, and never
   while it is still painting (its paint would land on the next owner). An
   earlier draft evicted anything not drawn on the current frame; with more
   dolls in view than cells, that repainted cells every frame. Now the dolls
   that have cells keep them. A colony with more than 64 distinct
   dolls in view at once falls back to 🧟 for the overflow. A doll that fails
   to paint stays on its fallback and is not retried.

Hover text and legends map a doll index to `dollFallback`'s text, so text
surfaces never learn that dolls exist. This mirrors how looks resolve into
`symbols`.

**The part library** (`web/src/map/dolls.ts`). Art lives in the frontend,
like [tile-art.md](./tile-art.md), because the wire only needs the key. Every
fragment shares one `viewBox="0 0 128 128"` and **one skeleton**: fixed
anchor points for head centre, shoulders, hips and feet, the same for all six
figures. That shared skeleton is the rule that makes the parts library small.
A tail drawn once at the hip anchor fits every body, so parts don't
multiply by figure.

The figure is a small full-length person in a colony jumpsuit, standing on
the cell's bottom edge, rather than the emoji's head-and-shoulders: a tail and
a third arm need a body to hang off. The jumpsuit's colour follows the figure
(blue, purple, green for man, woman, person) so gender still reads at a
glance, as it did with the emoji. Seniors get glasses and a couple of lines,
and keep their hair.

Layers, back to front:

1. tail (from the hip), vestigial twin (a small sleeping head behind the left
   shoulder)
2. back hair (long or bobbed figures)
3. body: arms, legs, boots, torso
4. third arm (waving from the right side)
5. head and face
6. front hair (none when bald), then a senior's glasses
7. extra eye, last, on the forehead over any fringe: it is the part people
   notice first, so nothing may hide it

Colours are plain fills chosen in TypeScript (a small palette per tone and
hair), not CSS variables, so the composed SVG is self-contained and decodes
the same everywhere. It carries its own `width` and `height` for the
Firefox/Safari reason [species-pack.md](./species-pack.md) records.

**State icons still win.** A colonist drawn by their state (😱 💬 🔫 📦 🚒) has
no look today, and in S2 has no doll either. While a mutant is panicking or
fighting, it draws as the state icon, at its scale. That keeps all
information currently on the map.

**Panels.** The Inspect heading shows the doll the map draws (the entity
topic's `doll`, from `DollKey`, so a fleeing mutant's heading is 😱, as the
map is). Roster rows show the colonist's resting doll whatever they are
doing (`RosterRow.Doll`, from `RestingDollKey`), as the roster's glyph has
always ignored passing states. Both are `<img>`s from `dollURL`, beside
`spriteURL` in `web/src/ui/sprite.ts`. The hover line shows the fallback.

**Testing it by eye.** Mutants are rare, and the mutation settings are not on
the New game form. To look at dolls in a running game, temporarily drop the
mutant test in `DollKey` (that is exactly S3) and `npm run wasm`; set a part
bit by hand in the same place to see the parts.

### S3: every colonist, and state badges

- `DollIndex` covers every colonist with a profile, not just mutants.
  Seniors get hair, which Unicode never allowed them. Looks remain for text
  surfaces and as the fallback before a doll paints.
- The pool needs to hold the colony's distinct (figure, tone, hair) set, which
  in practice is a few dozen. Size it from `Hello` (for example, the number of
  colonists at landing × 1.5, capped at 512).
- **State badges** replace the state-figure swap: the doll stays, and a small
  badge (the same emoji, at about 40% size) is drawn at the top corner in a
  second instance pass. Only then do state icons stop hiding the body. This
  is the one real UX decision in S3. Play it before committing to it.

## Why it is this way

- **Height on the wire, not a scale.** A scale would put a display rule in Go
  and every tweak would change golden frames. Centimetres are a fact the TUI,
  the inspector and a future frontend can all use.
- **A compressed curve, not linear.** At linear scale a 15 cm egg is 0.08
  tiles, invisible at every zoom, and a 640 cm queen is 3.6 tiles and covers
  a room. The 0.75 power keeps a two-step mutant visibly taller (1.5×) while
  keeping the extremes playable. Clamping a linear curve instead would
  push everything small to the floor, and an egg would look the same size as
  a grub.
- **Feet anchoring.** With centre anchoring, a small body floats in the
  middle of its tile and a big one sinks into the floor below. The map is
  viewed from the front, like the emoji, so the floor is the tile's bottom
  edge.
- **The lab's house style doesn't change.** Creature Lab's brief already
  tells artists to fill the tile and "show its smallness through its shape,
  not by drawing it small". That rule was written because the map could not
  scale. It still holds: scaling is now done by the renderer, so a sprite that
  also drew itself small would be shrunk twice.
- **Dolls are composed, not drawn per key.** With 2,880 keys, one drawing per
  key is not an option, which is what a Creature Lab-style "one SVG per slot"
  approach would need. Layers on a shared skeleton reduce the art to six
  bodies, a head per figure, six hair shapes and four parts.
- **A deterministic key space, not a table that grows.** The alternative was
  a doll table that grows mid-game, sent as Hello deltas when a new body
  appears. That needs a new message, ordering against frames, and save/restore
  of the table. Enumerating every key up front makes the index a pure
  function of the colonist, like looks, and only the page's cell pool is
  dynamic, and that pool is purely local.
- **Mutants first.** Mutants are where the payoff is (🧟 currently erases the
  colonist's look entirely), the set is small, and it tests the compositor,
  the cell pool and scaling before the change touches every colonist.

## Extending it

- **Per-specimen alien height.** Roll an individual height inside the
  species' range at spawn, on the simulation stream, since size already feeds
  HP and damage. Then send that height instead of the midpoint. This changes
  seeds, so it is a simulation change with its own doc update, not a display
  tweak.
- **A new mutant part:** add a bit to the parts digit (doubles the key space,
  still well inside `uint16`), a fragment at a skeleton anchor, and its layer
  slot. Append radixes and bits, never reorder them. Nothing persists doll
  keys across games, but the golden frames should only change when a doll is
  actually added.
- **A new hair or figure:** a new value in its digit. The key space grows
  multiplicatively, so check `len(All)+len(Looks)+len(Sprites)+keys` against
  `uint16` in a test, as looks do.
- **Cats, rats, chickens:** give the species a nominal height in its species
  table and send it. They draw at 1.0 until then.
- **Invariant:** every doll index has a fallback symbol, and the map never
  shows an empty cell for a colonist who exists.

## Related

- [colonist-looks.md](./colonist-looks.md): the emoji looks dolls replace on the map.
- [mutation.md](./mutation.md): the parts and stature dolls and scaling show.
- [species-pack.md](./species-pack.md), [creature-lab.md](./creature-lab.md): pack sprites, the atlas paint path, the lab's house style.
- [alien-lifecycles.md](./alien-lifecycles.md): `SizePct` per form.
- [frontend-web.md](./frontend-web.md), [wire-format.md](./wire-format.md): the atlas, the sprite pass, the entities section.
- [tile-art.md](./tile-art.md): the precedent for art living in the frontend.
