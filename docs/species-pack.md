# Species packs

> Part of the [mars-sim documentation](./README.md).

## What it is

A species pack is a file of alien species pulled from [Creature Lab](./creature-lab.md), each with an SVG sprite for every form of its life, plus the lab's writing about it. When the game has one, a world picks its alien roster from the pack instead of rolling one. In the browser, every alien of a packed species draws with its own form's sprite: the egg as the egg, the matriarch as the matriarch. The terminal stays on emoji. In both, the lore tab shows the lab's field notes and lab notes.

```sh
go run . -fetch-species https://creature-lab-b2mxg.sprites.app   # writes species-pack.json
go run .                                                          # plays from it
```

## Source

- [`internal/labpull/labpull.go`](../internal/labpull/labpull.go): `Pull` reads a lab's public API and builds the pack. `Encode` writes it, refusing an empty one. [`labpull_test.go`](../internal/labpull/labpull_test.go) runs it against a fake lab, and against a real one when Postgres is available.
- [`internal/sim/species_pack.go`](../internal/sim/species_pack.go): the file's types (`SpeciesPack`, `PackedSpecies`, `PackedSprite`, `SpeciesLore`), `LoadSpeciesPack`, `packedLore`, `packedRoster` (the draw), `alienSprites` (the world's sprite table), and `spriteFor` (which sprite an alien draws). [`species_pack_test.go`](../internal/sim/species_pack_test.go) covers loading, the roster, and the per-form sprites.
- [`internal/sim/world.go`](../internal/sim/world.go): `newWorld` takes the packed roster when `Config.SpeciesPack` holds any.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go): `EntityView.Sprite`, `Snapshot.Sprites`, and `Snapshot.AlienLore` with its readers `FieldNotes(i)` and `LabNotes(i)`.
- [`internal/wire/topics.go`](../internal/wire/topics.go) (`LoreSpecies.Description` and `Notes`), [`web/src/ui/LorePanel.svelte`](../web/src/ui/LorePanel.svelte) and [`internal/ui/tui/render_lore.go`](../internal/ui/tui/render_lore.go): the lore tab in the browser and the terminal.
- [`internal/wire/wire.go`](../internal/wire/wire.go): `HelloGlyphs.Sprites` and `SpriteFallback`, and the sprite range in `entityGlyph`. Tested in [`sprites_test.go`](../internal/wire/sprites_test.go).
- [`main.go`](../main.go): `-species-pack` (loaded before flags, like `-alien-names`) and `-fetch-species`.
- [`cmd/mars-sim-wasm/main.go`](../cmd/mars-sim-wasm/main.go): `start` takes the pack under a `species-pack` key (`hostAPI` 20).
- [`web/build-wasm.sh`](../web/build-wasm.sh) copies `species-pack.json` next to the page. [`web/src/main.ts`](../web/src/main.ts) (`speciesPack`, `withLooks`), [`web/public/worker.js`](../web/public/worker.js) and [`web/src/sim/client.ts`](../web/src/sim/client.ts) carry it to the engine. [`web/src/map/atlas.ts`](../web/src/map/atlas.ts) paints the sprites.

## How it works

### Pulling

`-fetch-species URL` calls `labpull.Pull`, which uses only the lab's public reads, so it needs no key:

1. `GET /api/species`: the catalog. Species still missing a sprite for some form are counted and left out.
2. For each complete species: `GET /api/species/{id}` (seed, generator commit, traits, forms), then `GET /api/species/{id}/candidates?svg=1` (the accepted SVGs).
3. **The lab's writing.** The species' **lab notes** (`notes`) always come along. Its **field notes** (`traits.description`) come along only when `descriptionEdited` says they were rewritten in the lab. Unedited, the game writes them itself from the species it actually plays, so they always match its stats.
4. **Re-roll and check.** The public reads describe a species but do not carry the stored `sim.AlienSpecies`; only the private `/api/export` does. So `Pull` rolls the seed again with `sim.RollLabSpecies` and keeps the species only if what it rolls *is* the lab's species: same names, scientific name, temperament, apex, and the same forms in the same order. Anything else is skipped with a line saying what disagreed ("rolled by e5d324640f6f as "zorbs"; this build rolls "grelks" from that seed").
5. The pack is checked by loading it (`Encode` runs `LoadSpeciesPack`) and only then written. A pull that finds nothing leaves the old file alone.

The file has the same shape as the lab's `/api/export`: `{source, exportedAt, species: [{id, seed, generatorRev, description, notes, species, sprites: [{form, name, svg}]}]}`. `description` and `notes` are each capped at 16 KB when loaded. An export saved to disk is therefore a pack too. A pack is about 8 KB per species.

### Playing

`main.go` reads `species-pack.json` from the working directory, or the file `-species-pack` names, into `Config.SpeciesPack`. That is the same convention as `alien-names.yaml`: an absent default file is normal, a named file that is missing or malformed stops the run, and `-species-pack ""` opts out. In `newWorld`, a non-empty pack replaces `rollAlienSpeciesRoster` with `packedRoster`:

- It draws `alien-species-count` species from the pack on the lore stream (`seed ^ alienLoreSeed`), in a seeded shuffle, so the same seed and pack give the same roster.
- It never puts two species with the same common or scientific name in one world. Two lab species can share a name; a world cannot.
- Asking for more species than the pack holds gives the whole pack. A pack **replaces** rolling rather than mixing with it, so every species in a world is one somebody looked at and drew.
- Bite damage, bite rest and slowness are worked out again from this world's settings (`alien-damage` and friends). The lab rolled them under the defaults. Everything else is used as packed, including lifecycles and stage lengths.

### Drawing

- `buildAlienSprites` lays the roster's sprites out in one list, with a table from (roster index, form) to a place in it. `spriteFor` looks an alien up by its species and `life.form` (form 0 for a single-form species), and the snapshot carries the result as `EntityView.Sprite` (index + 1; 0 means none). `Snapshot.Sprites` is the list itself, shared by every snapshot because it never changes.
- On the wire, the Hello carries `glyphs.sprites` (the SVG sources) and `glyphs.spriteFallback` (the plain alien's symbol). A frame's glyph index past the colonist looks, `len(symbols) + len(looks) + i`, means `sprites[i]`. The frame layout did not change: it is still a uint16 glyph index, so `wire.Version` stayed at 5.
- The page's `withLooks` appends the fallback symbol once per sprite, so the hover line and legends show a plain 👽 as text, and every index stays a plain index into `symbols`. `buildAtlas` draws the emoji, then decodes each SVG as an `Image`, paints it over its cell, uploads the texture again, and marks the map dirty. Before decoding, `sized` gives the SVG's root a 128×128 `width` and `height` if it lacks them. Until the images decode, or if one fails, the cell shows the emoji, and the console says which sprite failed and why.

### The lore tab

`packedLore` keeps each picked species' rewritten field notes and lab notes by roster index; the world holds them (so saves carry them) and `Snapshot.AlienLore` shares them. Both lore tabs read through `Snapshot.FieldNotes(i)`, which returns the rewrite when there is one and `AlienSpecies.Description()` otherwise, and `Snapshot.LabNotes(i)`. The lab notes appear as a **Lab notes** section under the field notes. Both texts keep their own line breaks: CSS `white-space: pre-line` in the browser, and `writeParagraphs` wrapping each line separately in the terminal. Rolled species, and packed species without lab notes, look as they did before.

In the browser, the lore tab also shows a packed species' sprites. The species list and the heading show its **portrait** in place of the emoji: the first form of its last stage, which is the adult or the first caste. Below the scientific name, a species with several forms gets a row of every form's sprite, named (grub, nymph, instar, adult). The lore topic carries these as `forms` (a name and an index into `Hello.glyphs.sprites`, from `Snapshot.FormSprites`) and `portrait`, plus `title`, the label without its emoji. Nothing in the topic repeats an SVG. The panel turns each index into an `<img>` with a `data:` URL, sized like the atlas's images (`sized` in `atlas.ts`). The terminal keeps the emoji.

### In the browser

`web/build-wasm.sh` copies the root `species-pack.json` into `web/public/` (gitignored there), or deletes a stale copy when there is none, so the GitHub Pages build ships whatever pack is committed. The Pages workflow lists `species-pack.json` among the paths that trigger a deploy, so a commit that only refreshes the pack redeploys the site. At each new game the page fetches `species-pack.json` from its own origin (once per page load; a 404 means no pack) and passes it to `start`. The worker merges it into the settings under `species-pack`, and the engine takes it back out before reading the settings, because it is not a setting.

## Why it is this way

- **Pulled ahead of time into a file, not fetched as a world starts.** A world is a pure function of its seed and settings ([determinism.md](./determinism.md)). A roster that depended on whatever the lab held at that moment would make a seed mean different things on different days, and a run with no network would break. A committed pack also ships with the Pages build, so players never reach the lab at all.
- **Re-roll and verify, rather than trusting a seed.** Rolling the same seed on a newer commit can give a different species; this is why the lab stores species whole. A seed alone could quietly attach a sprite to the wrong creature. Verifying against the lab's own traits catches every drift that changes what the sprite should show (names, temperament, apex, forms). Small numeric drift (a height range) would pass unnoticed, but it does not change a drawing.
- **Why not `/api/export`?** It returns the exact stored species, which would make the re-roll unnecessary, but it needs a key. Making it public (`security: []` in `creature-lab/api/openapi.yaml`) would let `Pull` take species exactly as stored, including ones rolled by an older build. The pack format already matches it.
- **A pack replaces rolling.** Mixing packed and rolled species raised questions with no good answer: which rolled species fill the gap, and how to keep their names from colliding with packed ones. "The world uses the pack, up to its size" is easy to predict and easy to explain.
- **Rewritten field notes replace the generated ones, but unedited ones are never copied.** A rewrite is somebody's chosen words and has to come from the lab. The generated text, though, is a pure function of the species, and the game's own copy describes exactly the stats it plays. A copy from the lab could disagree after any small drift in the roster code.
- **The lore tab shows the same art as the map.** Before it did, a packed species drew as its sprite on the map but as its emoji in the lore tab, so the two did not look like the same creature. The topic sends indexes rather than SVGs because the Hello already holds every sprite.
- **Lab notes get their own section rather than joining the field notes.** They are written in their own voice (an orientation sheet, a survivor's account) and read as a document found in the world, not as the encyclopedia entry above them.
- **Combat stats are recomputed; anatomy is not.** Bite damage and pace are scaled from settings a player can change, and a pack must not lock them to the lab's defaults. Anatomy, forms and names are the creature, and the sprite was drawn for exactly those.
- **Every sprite gets a size before it decodes.** The lab's house style gives a sprite only a `viewBox`, so it has no intrinsic size. Chrome makes one up (150×150) and draws it. Firefox, and some Safari versions, decode it with a natural size of 0, and `drawImage` then draws nothing. The first release did not size them: it worked in Chromium, where it was tested, but not for players on other browsers. A cell is now cleared only once its image is known to draw, and a failed re-upload (a browser that taints a canvas drawn with SVG) keeps the all-emoji texture rather than breaking the map.
- **Check a pull's summary.** A pack pulled by an older build silently lacks whatever that build did not know to pull: the first pack committed was pulled moments before lab notes were supported, and had none. `-fetch-species` now ends by counting the species with lab notes and with rewritten field notes, so a pull that brought no text is obvious.
- **The sprites draw as images, never as markup.** An SVG is text a model wrote. `loadSVG` decodes it through an `Image`, which never runs script, and nothing puts it into the page's DOM. The lab refuses active content too ([creature-lab.md](./creature-lab.md)), but the game does not rely on that.
- **The pack rides beside the settings, not inside them.** Settings are scalars (`mars-sim.yaml` keys); a pack is a 30 KB document. It travels as its own argument through `client.start` and the worker, and joins the settings mapping only at the WASM boundary, where `takeSpeciesPack` removes it again.
- **Saves.** The pack lives in `Config` and the sprite table in `World`, so a save carries both and plays on with the same creatures and art without the file. Adding the fields changed the save layout, so saves from before this change do not load ([save-load.md](./save-load.md)).

## Extending it

- **Terminal sprites:** not planned. The TUI draws one cell per tile, and an SVG cannot help there; packed species keep their emoji.
- **Public export:** to pull species exactly as stored, make `exportCatalog` public in the lab's spec, then have `Pull` try `/api/export` first and fall back to the re-roll. The pack format needs no change.
- **New fields on `AlienSpecies`:** a pack stores the struct as JSON, so a field added later loads as its zero value from an older pack. Give a new field a sensible zero, or recompute it in `packedRoster` as the combat stats are.
- **A different draw** (for example, weighting by temperament) belongs in `packedRoster`. Keep it on the `rng` it is given, so the roster stays a function of the seed.

## Related

- [creature-lab.md](./creature-lab.md): where the species and sprites come from.
- [lore.md](./lore.md), [alien-lifecycles.md](./alien-lifecycles.md): what a species and its forms are.
- [frontend-web.md](./frontend-web.md): the map's atlas. [wire-format.md](./wire-format.md): the Hello.
- [sprite-designer.md](./sprite-designer.md): drawing one sprite by hand.
