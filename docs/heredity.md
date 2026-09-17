# Heredity: shared surnames, inherited looks, and family warmth

> Part of the [mars-sim documentation](./README.md).

## What it is

Heredity is what joining a family does to a newly generated colonist beyond
placing them in the tree. A colonist born into a family takes that family's
**surname**, inherits their closest relatives' **appearance** feature by
feature, and starts out with **positive affinity** toward every relative rather
than at a stranger's zero. It runs at worldgen, over the kinship tree that
[ages-and-family.md](./ages-and-family.md) describes.

## Source

- [`internal/sim/heredity.go`](../internal/sim/heredity.go) — the pass itself: `inheritFamily`, `adoptSurname`, `closestKin`, `inheritAppearance`, `seedFamilyAffinity`, and the `kinBonds` table.
- [`internal/sim/personality.go`](../internal/sim/personality.go) — the heritable forms of each attribute (`hairBase`, `heightZ`, `given`/`surname`) and name uniqueness.
- [`internal/sim/relationships.go`](../internal/sim/relationships.go) — the tree the pass reads, and the affinity store it writes.
- [`internal/sim/world.go`](../internal/sim/world.go) — `spawn`, where the three generation steps are ordered.
- [`internal/sim/heredity_test.go`](../internal/sim/heredity_test.go) — the tests that pin all of it.

## How it works

### Three ordered steps, not one

Colonist generation in `spawn` is three steps, in this order:

1. `assignPersonality` — roll a whole, standalone person.
2. `assignKin` — find them a family, using that person's age and gender.
3. `inheritFamily` — rewrite the parts of them their family decides.

The order is forced, and it is the central design point. Heredity cannot run
inside personality generation because none of it can be decided before the
colonist's place in the tree is known. But the tree cannot be built first
either: `wireRelation` validates a proposed tie against a *finished* profile — a
parent must be at least twenty years older, a spouse must be a plausible match
by gender and orientation. So the person is rolled complete, then partly
overwritten.

That is also why every feature has a plausible rolled value to fall back on:
inheritance is probabilistic and per-feature, so a colonist who inherits only
their mother's hair still has a skin tone and a height of their own.

`assignKin` reports whether it actually tied the colonist to anyone, so a
colonist who arrived alone skips the pass entirely.

### `kinBonds`: one table, two dials

Every derived relation kind has an entry in `kinBonds`:

| Relation | `genes` | `warmth` |
| --- | --- | --- |
| spouse | 0 | 100 |
| parent / child | 100 | 90 |
| sibling | 100 | 80 |
| grandparent / grandchild | 50 | 65 |
| aunt/uncle / nibling | 50 | 55 |

`genes` is a percentage of `cfg.AppearanceInheritChance`; `warmth` a percentage
of the `cfg.FamilyAffinity` baseline. The config sets the overall strength and
the table keeps the relative shape, so retuning "how much do families resemble
each other" is one number and does not disturb the ordering.

A spouse passing **no** genes is doing double duty: it is both the biological
fact and the flag that marks someone as married in rather than born in, which is
what keeps a household's two surnames straight.

### Surnames

`adoptSurname` takes the surname of the **lowest-entity-ID blood relative** —
the first of that line to land on Mars. Any blood relative would usually do,
since each one took the line's name on the way in, but there is one case where a
colonist's relatives disagree: the children of a couple where one spouse kept
their own name are blood relatives of both parents. Anchoring on the earliest
arrival resolves that consistently, and it resolves it the way real families
look — the child carries one parent's name and the other parent is the odd one
out.

A colonist whose only tie is a spouse has married into the colony, and takes
their spouse's name only `SpouseSurnameChance` of the time.

So the invariant is *not* "everyone you are related to shares your surname" —
it is **"you carry the surname of the earliest-arrived member of your blood
line."** `TestGeneratedColonyFamilies` asserts exactly that.

### Appearance

`closestKin` returns the whole tier of relatives with the highest `genes`, not
one relative. `inheritAppearance` then draws a donor from that tier
*independently for each feature*, so a child of two colonists can have one
parent's hair and the other's skin instead of being a copy of whichever parent
was picked first.

Each feature is inherited in the form that actually passes down, which is not
always the displayed value:

| Feature | Inherited as | Why not the displayed value |
| --- | --- | --- |
| skin tone | the relative's tone ±1 step (50% exact, 25% each way) | an exact copy makes families look cloned; the nudge gives a family a *range* on the five-point scale |
| hair | `hairBase`, the natural color, never white or bald | white and bald are age showing, not genetics — a grandmother gone white passes down her brown |
| height | `heightZ`, standard deviations from the colonist's **own gender mean** | a 190cm father should give a tall *daughter*, not a 190cm one |

Two consequences fall out of the hair split. An inherited color only replaces a
natural one, so a colonist who has already greyed stays grey while their
`hairBase` still updates (and still passes on). And `rollHair` now returns both
values, with the age-driven white/bald roll applied on top of a natural color
rather than replacing it in the same draw.

Height uses `z*h + noise*sqrt(1-h²)` with `heightHeritability = 0.6`. The
weighting is not arbitrary: `0.6² + 0.8² = 1`, so the colony's overall height
distribution keeps its shape. Families vary less internally than the colony
does, without the colony as a whole creeping toward the mean as lines grow.
`setHeightZ` re-frames a colonist at the new z-score while **keeping the BMI
they were rolled with** — inheriting a frame should change how tall someone is,
not how heavy-set.

### Starting affinity

`seedFamilyAffinity` writes `FamilyAffinity` (a percent of `AffinityMax`) scaled
by the relation's `warmth`, plus a `FamilyAffinitySpread` jitter so relatives
aren't all at a flat number per kind. Family arriving on the same ship have
known each other for years; making them talk their way up from zero like
strangers was plainly wrong.

Affinity is stored per direction, but family warmth goes in through
`addAffinity`, which writes both directions equally. That is deliberate: a
shared history is a property of the pair, not something one relative can hold
more of than the other. One-sided forces (a Mutant-Lover's pull toward a
mutant, see [mutation.md](./mutation.md)) are what make the two readings
diverge later in play.

This writes simulation-visible state from the personality RNG stream — which is
what family generation already does. The draws stay off `World.rng`, so the
invariant in [`AGENTS.md`](../AGENTS.md) holds: a run with `FamilyChance = 0` is
bit-for-bit unaffected.

### Name uniqueness

Shared surnames pull whole families onto one name, which makes full-name
collisions much likelier — and the pools are small enough (24 given names
against 26 surnames) that a colony of thirty already collided about half the
time *before* this change. Two colonists with the same name are simply
indistinguishable in the roster, so `uniquifyName` re-draws the **given** name
until the full name is free, bounded at `givenNameRedraws` attempts. Only the
given name moves: the surname may be a family's, which is the part worth
keeping. It runs both on a freshly rolled name and after a surname is adopted.

The check reads `World.colonistNames`, an index of every living colonist's name,
rather than scanning the roster: worldgen is already quadratic in colonist count
(`assignKin` samples the whole roster per spawn), and a per-spawn name scan made
generating a 500-colonist colony about twice as slow. `releaseName` frees a name
when its holder dies or is about to be renamed, guarded on ownership so a rename
never evicts the namesake who was there first.

## Why it is this way

- **A separate pass, not a rewrite of personality generation.** Folding
  heredity into `assignPersonality` would mean generating a partial person,
  building the tree against it, and coming back — but `wireRelation` needs the
  age and gender that generation produces. Rolling a complete person and then
  overwriting part of them is the only ordering that works, and it has the nice
  property that every un-inherited feature already holds something sensible.
- **Heritable forms are stored separately from displayed ones.** The first
  attempt inherited `HairColor` and `HeightCM` directly. Both are wrong: a
  white-haired grandparent made white-haired grandchildren, and a tall father
  made daughters taller than any woman in the colony. Storing `hairBase` and
  `heightZ` alongside the displayed values fixes both, and costs two unexported
  fields on `Profile` that nothing outside generation reads.
- **Per-feature donors, not a single donor.** Picking one relative and copying
  their whole appearance produced visible clones. Drawing per feature is one
  extra line and mixes two parents the way you would expect.
- **Anchoring surnames on the lowest entity ID** rather than on the relative the
  colonist was wired to. The wired relative is arbitrary — a colonist attached
  as a grandchild of X is equally the sibling of Y — and using it let a line
  drift onto two names.
- **A skin-tone nudge rather than a copy.** Exact inheritance at 75% made
  families read as duplicates in the roster. The ±1 step keeps the resemblance
  legible while leaving room for variation.
- **`FamilyAffinity` as a percent of `AffinityMax`**, so raising the affinity
  range with `-affinity-max` does not silently turn family warmth into a
  rounding error.

Measured over 40 seeds of a 30-colonist colony: blood relatives share a natural
hair color 61% of the time against a 32% colony baseline, and sit a mean 1.03
skin-tone steps apart against a 1.61 baseline.

## Extending it

- **A new heritable feature**: add the heritable form to `Profile` (unexported,
  next to `hairBase`/`heightZ`) if the displayed value is not what passes down,
  then add one `if d, ok := donor(); ok { ... }` block to `inheritAppearance`.
  The per-feature donor draw comes free.
- **Retuning resemblance or warmth**: `AppearanceInheritChance` and
  `FamilyAffinity` for overall strength, `kinBonds` for the shape across
  relation kinds. A new `RelationKind` **must** get a `kinBonds` row — a zero
  `genes` entry would silently be treated as married-in and break surname
  sharing.
- **Invariants to preserve**: heredity draws only from `World.prng`, never
  `World.rng`; `inheritFamily` runs after `assignKin`, never before; and any new
  surname source must keep "the earliest-arrived blood relative wins", or a line
  will fork onto two names.

## Related

- [ages-and-family.md](./ages-and-family.md) — the kinship tree heredity reads, and the age rule that constrains it.
- [personality.md](./personality.md) — the profile heredity partly overwrites, and the RNG-stream invariant.
- [configuration.md](./configuration.md) — the four tunables and their flags.
- [mutation.md](./mutation.md) — the other thing that rewrites a colonist's
  body, and the one-sided affinity that makes the two directions diverge.
