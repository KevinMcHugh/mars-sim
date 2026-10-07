# Game settings view

> Part of the [mars-sim documentation](./README.md).

## What it is

A read-only list, at the bottom of the browser's **Game** tab, of every
setting the running game was started with: each `cfg`-tagged knob in
`sim.Config` (see [configuration.md](./configuration.md)), grouped under the
section headings `mars-sim.yaml` uses, its value beside its compiled default,
plus the seed. The new-game form sets only a handful of these (size,
colonists, seed, fog of war, auto zoning); this is how a player sees the
other three hundred without reading Go.

## Source

- [`internal/wire/config.go`](../internal/wire/config.go) — the `config`
  topic: `ConfigTopic`, its sections and settings, built from `sim.Knobs`.
- [`internal/wire/config_test.go`](../internal/wire/config_test.go) — every
  knob listed once, under a titled section, with value and default.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) —
  `Snapshot.Config`, a pointer to the World's `Config`.
- [`web/src/ui/GameSettings.svelte`](../web/src/ui/GameSettings.svelte) — the
  list, with a filter box and a "Changed only" box; included by
  `NewGamePanel.svelte`.

## How it works

- **The engine side.** Each `Snapshot` carries `Config *Config`, pointing at
  the World's own copy. The `config` topic (in `topicTable`) copies it and
  walks it with `sim.Knobs`, the same reflection over `cfg`/`sec`/`doc` tags
  that generates the flags and `mars-sim.yaml`. A knob with a `sec` tag starts
  a new section. Values go out as JSON numbers or booleans; defaults come from
  `DefaultConfig`, reflected once (`defaultKnobs`).
- **The page side.** `GameSettings` subscribes to `config` and lists each
  section as a `<details>`, shut by default (there are a few hundred
  settings), opening them all while the filter or "Changed only" narrows the
  list. A setting that differs from its default is marked and shows the
  default under its description. The whole block is a foldable `Section`
  (`game.settings`).
- **Timing.** The topic is rebuilt once a minute at most: settings never
  change within a game, and a new game or a load calls `Topics.Restart`,
  which sends it again at once.

## Why it is this way

- **Reflection, not a hand-written list.** The knob list already drives the
  flags and the yaml template; a third surface written by hand would drift
  the first time someone added a tunable. A new `cfg` field shows up here
  with no UI change.
- **A pointer on the Snapshot, not a copy.** `Config` is large (drive and
  focus spec arrays, the cognition tables), and snapshots are taken every
  frame. A World's `Config` is never written after `newWorld` (only read,
  sometimes through a pointer like `&w.cfg.Drives[d]`), so sharing it with a
  frontend goroutine is safe. If something ever starts mutating `w.cfg`
  mid-game, this has to become a copy.
- **"Default" means `DefaultConfig`, not the browser's form defaults.** The
  page's own new-game defaults (10000×10000, for instance) are not the
  engine's (80×40), so width and height always read as changed. That is
  accurate: the browser reads no `mars-sim.yaml`, so every setting the form
  does not send is `DefaultConfig`'s, or whatever a loaded save carried.
- **Read-only on purpose.** Making all of these settable from the page is a
  bigger question (validation lives in `main.go`'s `validateConfig`, and many
  knobs only make sense together). Seeing them comes first.
- **Not shown:** untagged parts of `Config` — the director's `Schedules`,
  the cognition tables from `cognition.yaml`, alien names. They are not
  scalar knobs; they would need their own views.

## Extending it

- A new scalar knob needs nothing here. A new knob *type* (a float, a
  string) needs a case in `knobValue` as well as in `bindConfigFlags` and
  `sim.assign`.
- Making a setting editable belongs in the new-game form (`NewGamePanel`),
  which sends `mars-sim.yaml` keys to `start`; this list will show it as set.

## Related

- [configuration.md](./configuration.md) — `sim.Config` and the knobs.
- [config-file.md](./config-file.md) — the tags and the yaml template.
- [wire-format.md](./wire-format.md) — topics.
- [frontend-web.md](./frontend-web.md) — the Game tab.
