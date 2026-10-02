# Arms makers

> Part of the [mars-sim documentation](./README.md).

## What it is

The second piece of lore after the alien species roster: every world rolls a
handful of corporations back home (`corporation-count`, default 4), and every
gun the colony can hold — pistol, shotgun, assault rifle — gets a maker from
that roster and a model name in the maker's house style. Combat narration
names the gun by it ("Zoe Vargas guns down a grelk with a MarsCorp M-117
shotgun."), and both lore tabs list the guns and the companies. Corporations
are conversation lore colonists talk about, and most arriving colonists used
to work for one ("Worked as a drill operator for MarsCorp."). All of it is
flavor only: a model never changes a gun's stats, and a backstory never
changes what a colonist does.

## Source

- [`internal/sim/arms_makers.go`](../internal/sim/arms_makers.go) —
  `Corporation`, `GunModel`, `rollCorporationRoster`, `rollGunModels`, the
  name/HQ pools, the model-numbering schemes, `Corporation.Description`, and
  `World.gunModelFor`/`weaponPhrase`.
- [`internal/sim/arms_makers_test.go`](../internal/sim/arms_makers_test.go) —
  determinism, roster invariants (unique names and models), count clamping,
  and the narration phrase. `go test ./internal/sim -run ArmsLoreSample -v`
  prints a few seeds' rosters to eyeball.
- The same file's backstory half — `employerRoles`, `rollEmployer`,
  `employerOf`, `backstory` — and `Entity.employer`/`employerRole`, set in
  `ship.go` right after `rollBackground`; `EntityView.Backstory` carries it.
- `internal/sim/topics.go` — `corporationLore` and `LoreCorporation`, and the
  "old employer" wording in `conversationText`.
- `internal/sim/combat.go` — `shoot`, which narrates with `weaponPhrase`.
- `internal/ui/tui/render_lore.go` — `writeArmsLore`, the GUNS and
  CORPORATIONS sections under the species list.
- `internal/wire/topics.go` (`LoreGun`, `LoreCorporation`) and
  `web/src/ui/LorePanel.svelte` — the same on the web frontend.
- The backstory line in the inspectors: `detailLines` in
  `internal/ui/tui/render_roster.go`, `ColonistDetail.Backstory` in
  `internal/wire/inspect.go`, and `web/src/ui/InspectPanel.svelte`.

## How it works

`newWorld` rolls the roster right after the alien species, on a stream of its
own (`newRand(cfg.Seed ^ armsLoreSeed)`), then rolls one `GunModel` per entry
of `weaponKinds` (`Pistol`, `Shotgun`, `AssaultRifle`, in that order) off the
same stream. Both land on `World` (`corporations`, `gunModels`) and are copied
into `Snapshot.Corporations`/`GunModels`.

A corporation is a root drawn without replacement from `corporationRoots`
("Mars", "Halvorsen", ...) plus a suffix ("Corp", "Dynamics", "& Sons", ...).
A *glued* suffix joins without a space and makes the model code the root's
first letter (`MarsCorp` → `M`); otherwise the code is the name's initials
(`Ares Heavy Industries` → `AHI`). Each corporation also rolls an HQ (stored
with its preposition, "in Lagos" / "on Phobos"), a founding year, and one of
four house numbering schemes:

| Scheme | Example |
| --- | --- |
| code-number | `M-117`, `HD-7` |
| "Model" | `Model 12` |
| "Mk" + Roman numeral | `Mk IV` |
| code-number-letter | `KC-35C` |

Each gun kind picks its maker uniformly, so one company can make two or all
three guns (in its one house style — they read as siblings) and some make none;
`Description` says "None of its products made it to the colony" for those.

`World.weaponPhrase(kind)` is what narration uses: article, make, model and
kind ("an Ares Arms AA-9 pistol"), falling back to the bare kind ("a pistol")
when no model is rolled for it.

### Conversation lore

Each corporation is a `LoreItem` (`corporationLore`, ID `corporation:<i>`,
topic phrase its name), listed in `World.loreItems` after the species. A
conversation about one reads "Had a conversation with Bo about MarsCorp." —
or, when the speaker used to work there, "... about MarsCorp, my old
employer." (speaker) / "... MarsCorp, Ada's old employer." (listener).

### Backstory: a former employer

When a ship lands, each passenger rolls its skills (`rollBackground`, see
[skills.md](./skills.md)) and then `rollEmployer`: with
`corporation-employee-percent` (default 60) it gets a former employer from
the roster and a job there. The job comes from `employerRoles` for the skill
the colonist arrived known for — a smith was "a line machinist" or "a
test-range armorer", a colonist with no profession "a janitor" or "an
intern". `backstory(e)` renders "Worked as a metallurgist for Tharsis
Armory.", which the TUI and web inspectors show under the colonist's looks.
Colonists born in the colony have none.

## Why it is this way

- **Its own RNG stream.** Same reasoning as species (see
  [lore.md](./lore.md#where-it-rolls-and-on-what-stream) and
  [rng-streams.md](./rng-streams.md)): rolling arms lore must not shift any
  `w.rng` draw, so the golden hashes did not move. It also does not share the
  species stream, so changing the corporation pools never re-rolls a seed's
  aliens. It needs no saved state — it is a pure function of the seed, rolled
  once in `newWorld`.
- **Per gun kind, not per individual gun.** Items are counted stacks of an
  `ItemKind` with no per-item identity, so "this rifle is an M-117 and that
  one a Mk IV" has nowhere to live. One model per kind per world matches
  that, and matches the setting: the colony's rifles are machined at its gun
  bench to one licensed design.
- **Pistols and shotguns too, not just the bench-made rifle.** Only the rifle
  is produced on Mars, but crash-pod and supply-drop guns were produced
  somewhere; naming all three keeps narration consistent.
- **The kind stays in the phrase.** "with a MarsCorp M-117" alone would hide
  whether that was the pistol or the rifle, which matters to a player reading
  a fight. The extra word is worth it.
- **Uniqueness by bounded re-roll.** A clash needs one maker to draw the same
  model twice; re-rolling up to eight times on a stream nothing else reads is
  free, and a trailing letter breaks any tie left after that.
- **Backstory is a seed-and-ID hash, not a stream draw.** It is flavor, so
  AGENTS.md would put it on the personality stream — but any new draw there
  shifts every later personality draw (names, looks) for every seed. Hashing
  `Seed ^ ID` through `splitmix64` with its own salt, the way
  `arrivalRareItem` already does, spends no draw anywhere, needs no saved
  stream state, and leaves every other roll alone. Golden hashes did not move.
- **Stored on the entity, not recomputed.** The job matches the profession
  the colonist *arrived* with; recomputing from the current profession would
  rewrite a colonist's past each time it ranked up in something new.
- **Corporations as lore did not move the golden hashes either.** More lore
  items change which item the topic stream's one `IntN` picks, never how many
  draws it makes, and lore topics have no mechanical effect (only gossip
  does).
- **"Old employer" is wording only.** Weighting a colonist toward talking
  about its old company would be a nice touch but would change topic draws;
  the ask was flavor, so the pick stays uniform.
- **`ItemKind.String()` is unchanged.** It has no `*World`, and inventories,
  storage, the market and traces all use it as the goods name. Only combat
  narration (which has the world) says the model.

## Extending it

- **More names.** Add to `corporationRoots`, `corporationSuffixes`,
  `corporationHQs`. Expect seed-pinned rosters to change (only
  `TestArmsLoreSample` output, nothing golden). Keep roots capitalized ASCII
  — `initials` reads the first byte.
- **A new gun kind.** Append it to `weaponKinds`; it gets a model
  automatically and narration picks it up via `weaponPhrase`.
- **Model names elsewhere.** The inventory/storage panels and the market still
  say "assault rifle"; showing the model there means passing
  `Snapshot.GunModels` to the frontend's item labels.
- **Corporations that do more than make guns** — a line of business, a
  supply-drop sponsor in the director's narration.
- **More jobs.** Add to `employerRoles`; a new skill wants its own list (an
  empty one falls back to the `SkillNone` list).
- **Backstory with effects** — an ex-armorer liking the colony's rifles, two
  ex-colleagues starting with some affinity — would make it gameplay. That is
  a real change: move it off "flavor only" in this doc and test it like any
  other mechanic.
- **Gameplay effects** (a maker known for reliability, a model with better
  range) would turn this from flavor into balance: it is already on a
  deterministic stream, so that is allowed, but it then belongs alongside the
  weapon tunables in [combat.md](./combat.md).

## Related

- [lore.md](./lore.md) — the species roster and the lore pattern this follows.
- [combat.md](./combat.md) — weapons, `shoot`, and the death-cause phrase.
- [foundry.md](./foundry.md) — the gun bench that machines the rifle.
- [ships.md](./ships.md) — crash-pod pistols and shotguns.
- [configuration.md](./configuration.md) — `corporation-count`,
  `corporation-employee-percent`.
- [conversation-topics.md](./conversation-topics.md) — the `LoreItem`
  interface corporations implement.
- [skills.md](./skills.md) — the arrival background the job is chosen from.
