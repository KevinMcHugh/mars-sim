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
- `internal/sim/rooms.go` — `refreshSpatial` sorts the dirty-chunk list.
- `internal/sim/hpa.go` — `sortedLinks` orders abstract-graph expansion.
- `internal/sim/facilitychoice.go` — `chooseFacility`, the nearest-facility
  scoring (`compareFound` is its total order).

## How it works

**Go randomises map iteration order on purpose.** `for k := range m` yields a
different order on every run of the same binary. That is a feature — it stops
code depending on an order the language does not promise — and it is the single
largest source of nondeterminism in an engine like this one, because the world
keeps most of its indexes in maps: `w.entities`, `w.regions`, `w.dirtyChunks`,
`w.facilityTiles`, `w.storageContainers`, `w.board.frontier`, `region.links`.

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

### The lockstep test

`TestDeterministicRunAgreesEveryTick` builds two worlds from one seed, steps them
together, and compares a fingerprint after every tick. It fails on the first tick
that disagrees and names the first field that moved, checked in a fixed order:

    tiles → regions → rooms → frontier → cleaning → entities

The order is the point. A determinism bug shows up in the *entities* field
eventually — a colonist standing somewhere else — but by then it is hundreds of
ticks downstream of the cause. Seeing `field "regions"` at tick 7 instead says
"region labelling", which is a five-minute search rather than a five-hour one.

The grid layers are FNV-hashed rather than rendered; at one entry per tile per
tick, formatting them dominated the test's runtime, and the field name is the
whole diagnosis anyway.

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

That loop is gone now (see [needs.md](./needs.md)): candidates are collected
into a slice with their distances and sorted by `compareFound`, distance then
`lessPoint`. The two map loops left, `anyFreeFacility` over `w.facilityTiles`
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
  enough, wrap `World.rng`'s source in a recorder that captures a stack trace per
  draw and diff the traces: an identical RNG trace with divergent state proves
  the cause is ordering, not randomness, and narrows it to one call site.
- **What the fingerprint does not cover**: affect, memories, relationships, and
  inventories. Add them if a bug lands there; they were left out because every
  divergence found so far surfaced in position or labelling first.

## Related

- [personality.md](./personality.md) — the two RNG streams and why flavor must
  not perturb the simulation.
- [pathfinding.md](./pathfinding.md) — regions, rooms, and the abstract search
  whose tie-breaks depend on this.
- [spatial-index-and-performance.md](./spatial-index-and-performance.md) — the
  chunk index and job board, both map-backed.
