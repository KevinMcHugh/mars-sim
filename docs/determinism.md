# Determinism

> Part of the [mars-sim documentation](./README.md).

## What it is

One seed, one simulation. Two runs of mars-sim started from the same `Seed` and
the same `Config` must produce the same colony, tick for tick, forever — same
positions, same jobs, same deaths, same region labels.

This is a stated project invariant (see [`AGENTS.md`](../AGENTS.md)), not a
nice-to-have: reproducing a bug, bisecting a regression, and comparing two
tunings all depend on it. A save file is a seed plus a tick count only if it
holds.

This doc is about the *other* half of the invariant. The RNG half — flavor on
`World.prng`, simulation on `World.rng` — is covered in
[personality.md](./personality.md). This one is about everything that can make
two runs differ while the random numbers are identical.

## Source

- `internal/sim/sim_test.go` — `TestDeterministicRunAgreesEveryTick`, the
  lockstep regression test, and `worldFingerprint`.
- `internal/sim/golden_test.go` — `TestGoldenWorldHash`, which checks what
  fixed seeds produce against hashes pinned in
  `internal/sim/testdata/golden-hashes.txt`.
- `tools/determinism-check.sh` — runs the golden hashes natively, under amd64
  (Rosetta) and under js/wasm in Node.
- `internal/sim/rooms.go` — `refreshSpatial` sorts the dirty-chunk list.
- `internal/sim/hpa.go` — `sortedLinks` orders abstract-graph expansion.
- `internal/sim/facilitychoice.go` — `chooseFacility`, the nearest-facility
  scoring (`compareFound` is its total order).

## How it works

**Go randomises map iteration order on purpose.** `for k := range m` yields a
different order on every run of the same binary. That is a feature — it stops
code depending on an order the language does not promise — and it is the single
largest source of nondeterminism in an engine like this one, because the world
keeps most of its indexes in maps: `w.entities`, `w.regions`, `Layer.dirtyChunks`,
`Layer.facilityTiles`, `Layer.storageContainers`, `Layer.board.frontier`, `region.links`.

Iterating a map is fine. What is not fine is letting the order decide anything.
There are three shapes this takes, and only the third is obvious:

| Shape | Safe? | Example |
| --- | --- | --- |
| The loop's result does not depend on order (a sum, a count, an "any match", a min under a total order) | Yes | `nearestOfKindAnywhere` — min by distance, ties by entity ID |
| The loop assigns identity in visit order (a counter, an append) | **No** | `refreshSpatial` handing out `RegionID`s |
| The loop picks a winner under a comparison that is not a total order | **No** | `chooseFacility` (see below) |

The rule of thumb: **iterate a map only to compute something order-independent,
or sort its keys first.** Sorting a handful of chunk indices or region IDs costs
nothing next to the work the loop then does.

### Levels

The world is a stack of levels (see [layers.md](./layers.md)), and every rule
above holds level by level, with the level as the leading key: `lessPoint`
orders by level first; `refreshSpatial` re-floods dirty chunks level by level,
shallowest first, so region IDs still come out the same; every loop over
levels (`eachLayer`, `eachContainer`, `eachFacility`) runs in level order; and
the list of stairs is kept sorted. A deeper level's worldgen streams and scum
growth mix in its distance from the landing level, and the landing level
mixes in nothing, so it is bit-for-bit what it was before there were levels.
`TestStairRunsAreDeterministic` runs two colonies that dig down in lockstep.

### The lockstep test

`TestDeterministicRunAgreesEveryTick` builds two worlds from one seed, steps them
together, and compares a fingerprint after every tick. It fails on the first tick
that disagrees and names the first field that moved, checked in a fixed order:

    tiles → regions → rooms → frontier → cleaning → property → entities

`property` is the economy's state: the treasury and every wallet, each
fixture's owner and access, and every storage ledger line (see
[money.md](./money.md) and [property.md](./property.md)). Who got paid and
whose ore is in a chest must be as seed-stable as where anyone stands.

The order is the point. A determinism bug shows up in the *entities* field
eventually — a colonist standing somewhere else — but by then it is hundreds of
ticks downstream of the cause. Seeing `field "regions"` at tick 7 instead says
"region labelling", which is a five-minute search rather than a five-hour one.

The grid layers are FNV-hashed rather than rendered; at one entry per tile per
tick, formatting them dominated the test's runtime, and the field name is the
whole diagnosis anyway.

### Save and load

A loaded game must play on exactly as the saved one would have.
`TestSaveLoadPlaysOnIdentically` checks this the same way as the lockstep test
above. It saves and loads a world, steps the original and the copy together,
and compares their whole encoded state, not just a fingerprint. When they
part, it reports the same fingerprint fields. See
[save-load.md](./save-load.md).

### Golden hashes and other machines

The lockstep test compares two runs in one process, so it can never see a
seed that plays out differently on another machine: both runs share the CPU,
the compiler and the Go version. `TestGoldenWorldHash` compares against
pinned hashes instead. Each case pins a hash of the generated world at tick 0 and
again some ticks later: every tile's terrain, composition and discovery; every
entity's kind, position, HP, state and species, in ID order; refuse totals; and
the simulation stream's PCG state. The RNG state is the catch-all: a draw
added, lost or reordered anywhere moves it before anything visible diverges.
Needs, affect, inventories and projects are not hashed directly; they reach
the hash through the draws and positions they cause.

`tools/determinism-check.sh` runs it three ways: natively, as amd64 under
Rosetta on Apple silicon, and as js/wasm under Node, which is the browser
target. The usual ways a Go simulation drifts between machines are:

- **Fused multiply-add.** The Go spec lets an implementation fuse `x*y + z`
  into one FMA, which rounds once instead of twice. gc does this on arm64,
  ppc64, s390x, riscv64 and loong64, and on amd64 when built with
  `GOAMD64=v3` or higher; the default amd64 (v1) and wasm do not. Float
  arithmetic that feeds a decision can therefore round differently on a Mac
  and a PC. Keep gameplay geometry in integers, or force the rounding with an
  explicit conversion, `float64(x*y) + z`.
- **Map order**, above.
- **`int` size.** `int` is 64 bits on every target we build (wasm included),
  but hash and seed code should still use explicit `uint64`.

On a Linux amd64 machine (typical CI) the script can only run amd64 and
wasm, neither of which fuses, so it has to run on an arm64 host (any Apple
silicon Mac) to cover FMA.

The `lazy-1000x1010` case runs a big map with a halo of 1 and must generate
chunks after tick 0, so it covers generation during play. Every hash includes
the set of generated chunks: which chunks exist is part of the world, and it
must come out the same everywhere (see
[worldgen-chunks.md](./worldgen-chunks.md#when-chunks-are-generated)).

The pinned hashes change whenever a change is meant to alter what seeds
produce, which is most gameplay changes: the RNG state alone moves with any
new draw. Re-pin them in that change with

    go test ./internal/sim -run TestGoldenWorldHash -update

and say so in the commit message. `-update` rewrites
`testdata/golden-hashes.txt` from this machine's run. A `-run` filter
re-pins only the cases it selects and keeps the rest. The hashes used to be
string constants in the test, copied in by hand on every seed break, which
was tedious enough to make the test feel like a cost with no benefit. A
golden mismatch in a change that did not mean to break seeds (a refactor, a
speed-up) is the bug: do not re-pin it away. And re-pin on a machine whose
results you trust, ideally followed by `tools/determinism-check.sh`, since
`-update` writes whatever this platform produced. The
`caves-300x150` case also insists that its run breaks into a cavern, so a
re-pin cannot silently drop coverage of the breach flood. It checks
`World.cavernBreaches` rather than `hiddenFloor`, because hidden floor can go
up as well as down once chunks are generated during play.

## Why it is this way

The three bugs this test was written for were all live in `main`, and none of
them was caught by the pre-existing `TestDeterministicRun` — which compares a
single floor count after 300 ticks, a number two divergent colonies match
easily.

### Region IDs were assigned in map order

`refreshSpatial` iterated `w.dirtyChunks` (a `map[int]struct{}`) and re-flooded
each chunk in that order. A region's ID is just `rid := w.nextRegion; w.nextRegion++`
— so the IDs themselves came out in a different order on every run.

That is the worst kind of nondeterminism, because region IDs are *load-bearing*:
a room's ID is the smallest `RegionID` in its component, and the abstract search
breaks ties toward the lower ID. Both of those are careful, deliberate
tie-breaks, and both were breaking ties on a number that was itself random. The
fix is one `slices.Sort(dirty)`.

The lesson generalises: a tie-break is only as deterministic as the values it
compares. Writing `if a.ID < b.ID` looks like determinism and is not, on its
own.

### `chooseFacility` scored a farther facility as a tie

The nearest-facility loop computed each candidate's distance like this:

```go
d := bestDist                     // seeded from the running best
for _, n := range neighbors8 {
    if nd, ok := reached(fac.Add(n.X, n.Y)); ok && nd < d {
        d = nd
    }
}
if d < bestDist || (d == bestDist && lessPoint(fac, best)) {
    best, bestDist = fac, d
}
```

Seeding `d` from `bestDist` is an optimisation that quietly changes the answer.
A facility *farther* than the current best never beats `bestDist` on any of its
access tiles, so `d` stays exactly at `bestDist` — and then reads as an exact
tie, where `lessPoint` hands it the win for having a smaller coordinate.

So the loop's outcome depended on which facility `w.facilityTiles` yielded first,
and worse, it was wrong either way: a colonist would sometimes walk past a near
sink to a far one. Seeding `d` at "unreached" instead fixes both. This is the
pattern worth remembering — the comparison `(d == bestDist && lessPoint(...))`
*looks* like a proper total order, and is one only if `d` is computed
independently of `bestDist`.

That loop is gone now (see [drives.md](./drives.md)): candidates are collected
into a slice with their distances and sorted by `compareFound`, distance then
`lessPoint`. The two map loops left, `anyFreeFacility` over `Layer.facilityTiles`
and `committedUsers` over `w.entities`, compute an "any match" and a count: the
order-independent shape.

### `region.links` expansion order chose the corridor

`abstractCorridor` keeps a predecessor only on a strictly lower `g`, so when two
regions reach a third at equal cost, the one expanded first wins and lands in the
corridor. `region.links` is a map, so "first" was random. `sortedLinks` sorts
into a reused buffer.

This one is *not* currently observable in play, and it was still worth closing: a
corridor only constrains the tile search, which usually finds the same optimal
route inside a slightly different set of regions. A latent order dependence that
is invisible today is exactly how the other two got in.

### `tryAssignCraft` recorded its answer from inside a filter

`nearestScumhouse(e, ok)` ranges over the scumhouse set (a map) and keeps the
nearest candidate that passes `ok`. `tryAssignCraft`'s `ok` also *recorded*
which recipe and whose inputs it found, as a side effect. Every candidate runs
through the filter, so the recorded recipe was whichever candidate the map
yielded last, not the one chosen. With one scumhouse that was invisible. With
several, a cook could be sent to one kitchen with another's recipe,
differently on each run: a 40-colonist colony starved a different number of
people every time its seed was replayed. The filter now only answers yes or
no, and the recipe is worked out for the scumhouse actually chosen
(`craftableRecipe`). `TestDeterministicRunUnderScarcity` runs the lockstep
check with several kitchens.

The general rule: **a filter or comparator passed to a search over a map must
be pure.** Anything it writes is written in map order.

## Extending it

- **Adding a map to `World`**: before you iterate it, decide which of the three
  shapes above the loop is. If it assigns identity or picks a winner, sort the
  keys.
- **Adding a tie-break**: check that both sides of the comparison are themselves
  deterministic, and that the losing side's value was computed independently of
  the winner's.
- **Hunting a new divergence**: run two worlds in one process and diff them per
  tick — that is what the regression test does, and it beats hashing whole runs,
  which only tells you *that* they differ. If the fingerprint is not specific
  enough, point `World.rng` at a recorder wrapping `w.rngSrc.sim` that
  captures a stack trace per draw and diff the traces: an identical RNG trace with divergent state proves
  the cause is ordering, not randomness, and narrows it to one call site.
- **Changing what a seed produces on purpose**: re-pin with
  `go test ./internal/sim -run TestGoldenWorldHash -update` and run
  `tools/determinism-check.sh` before committing, so the new hashes are
  known to agree on every target.
- **Adding or renaming a golden case**: edit `goldenCases`, then run
  `-update` to pin it. Until it is pinned, the case fails with "no hash
  pinned".
- **What the fingerprint does not cover**: affect, memories, relationships, and
  colonist inventories. Add them if a bug lands there; they were left out because every
  divergence found so far surfaced in position or labelling first.

## Related

- [personality.md](./personality.md) — the two RNG streams and why flavor must
  not perturb the simulation.
- [rng-streams.md](./rng-streams.md) — every RNG stream, its seed, and how its
  state is saved.
- [pathfinding.md](./pathfinding.md) — regions, rooms, and the abstract search
  whose tie-breaks depend on this.
- [spatial-index-and-performance.md](./spatial-index-and-performance.md) — the
  chunk index and job board, both map-backed.
