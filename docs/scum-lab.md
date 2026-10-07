# Scum Lab

> Part of the [mars-sim documentation](./README.md).

## What it is

Scum Lab is the tuning bench for `cognition.yaml`: one shell, one in-memory config, a nav of tools. **Focus Tester** explains one colonist's focus. **Grammar Builder** edits perceptions, reactions, and trait rules. **Sprite Designer** draws SVG map sprites with Claude and does not touch the config; see [sprite-designer.md](./sprite-designer.md). The page does not run a world. Go still validates the file at load.

Focus scoring and colonist rolls go through a WASM build of the sim. Do not reimplement them in JavaScript. See [wasm.md](./wasm.md).

## Source

- [`tools/scum-lab/`](../tools/scum-lab/) — the page. `shell.js` is hash routing and the YAML dialog. `tools/registry.js` is the tool list. `shared/store.js` is the one config. `shared/sim.js` talks to WASM. `shared/yaml.js` reads and writes the file the lab emits. `shared/defaults.js` is the reset snapshot.
- [`tools/scum-lab/brand/`](../tools/scum-lab/brand/) — `logo.svg` (masthead) and `favicon.svg`. The favicon is a separate, simpler drawing on a 32px grid: the logo's bubbles and thin strokes blur into mush at 16px. Colors are the theme's amber and `--bg` rust; keep it free of white outlines.
- [`tools/scum-lab/build.sh`](../tools/scum-lab/build.sh) — builds the module. `--open` serves port 8765 and opens the page.
- [`cognition.yaml`](../cognition.yaml) — what the tools import and export.

## How it works

```sh
tools/scum-lab/build.sh
tools/scum-lab/build.sh --open
```

It is also published at <https://kevinmchugh.github.io/mars-sim/scum-lab/>. The Pages workflow ([frontend-web.md](./frontend-web.md)) runs `build.sh` and copies the folder, minus `wasm/` and `build.sh`, into the game's `web/dist`. The lab stays outside Vite on purpose: every URL in it is relative (`sim.js` finds the module from `import.meta.url`), so it works from any subpath as is. Keep it that way; a root-absolute URL breaks it under `/mars-sim/`. The game's Game tab links to it at the bottom.

ES modules do not load from `file://`. The shell mounts `#focus` or `#grammar` into `#tool`. Import, export, vocabulary, and reset live on the shell. Reset restores `defaults.js`, not an empty file. After import or reset the shell remounts the tool, so read config at `mount` time.

Focus calls `evaluate` / `rollColonist` in `shared/sim.js` on each paint. Grammar does not. The bench gate (no pod, toilet, bed, or person nearby) runs after the real scores and only clears eligibility. `chooseFocus` does not know about it. Need pressure stays on the bar when a later gate knocks the focus out. The sentences under the bars narrate those facts.

`LabRoll` builds PCG streams with `newPCG` and the same keys as `World.prng` and `World.agePRNG`. It is not colonist N of a world seed: worldgen spends those streams on family and heredity first, and the bench rolls one person on a fresh pair of streams.

## Why it is this way

Focus and grammar are two jobs: a person ("why did she run?") and a file ("what does `saw-alien` do?"). A third tuner had nowhere to go on one scroll. Native ES modules stayed; the WASM file is the only build step, and it exists so the bench cannot drift from `fillFocusCandidates`, `selectFocus`, and `rollProfile`.

Only traits are editable on the generated colonist. Name, age, and body are flavor the sim does not read back. A half-editable biography reads as if the lab could rename someone inside a run.

Traits split into **Native** (rolled at spawn) and **Acquired** (gained in play, like Mutant). The split comes from each trait's `acquired` flag in the catalog: a group is Acquired only when every trait in it is. A new acquired trait lands in the right section with no lab change.

## Extending it

1. Add `tools/scum-lab/tools/<id>.js` exporting `{ id, title, summary, mount(host, ctx) }`. `id` is the hash. `title` is the nav label. `mount` fills `host` and returns an unmount function.
2. Import it in `tools/registry.js` and append it to `tools`. Order is nav order.
3. Mutate `ctx.store.config` in place. No private copy, no second YAML dialog. `ctx.openYAML("import" | "export" | "vocab")` opens the shell dialog.
4. A one-off stays in the tool file. `shared/` is for logic a second tool needs.
5. When `cognition.yaml` defaults change, update `shared/defaults.js` in the same change. The lab does not regenerate it. Drift shows up as reset disagreeing with `go run . -print-cognition-config`.

A new *system* still uses this shell. Scoring still goes through the WASM API in [wasm.md](./wasm.md).

## Related

- [sprite-designer.md](./sprite-designer.md) — the Sprite Designer tab.
- [wasm.md](./wasm.md) — the browser module, and why a `mind` package is the next cut.
- [cli.md](./cli.md) — `-cognition`, `-print-cognition-config`, and `-print-cognition-vocab`.
- [compositional-perception-and-events.md](./compositional-perception-and-events.md) — the grammar `cognition.yaml` holds.
- [personality.md](./personality.md) — the roll `LabRoll` calls.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — the score the bench is explaining.
