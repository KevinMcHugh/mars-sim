# Colonist memories

## What it is

Colonists retain a short history of notable experiences. Completing a meal,
conversation, mining or construction task, and seeing an alien or mouse creates
a memory; walking and idling do not. Memories are part of the immutable
`EntityView` in a `Snapshot`, so frontends can show a colonist's history without
access to live simulation state.

## Source

- `internal/sim/entity.go` defines `Memory` and the colonist storage.
- `internal/sim/world.go` owns the bounded append operation.
- `internal/sim/systems.go` records experiences at their completion or sighting.
- `internal/sim/snapshot.go` deep-copies memories for consumers.

## How it works

Each memory has the simulation tick and a human-readable description. The
simulation keeps the newest 64 entries per colonist. Creature sightings are
edge-triggered: remaining near one creature does not add a new entry every
tick, but leaving and seeing it again does. The history is copied into
`EntityView.Memories`, along with other colonist-only data.

## Why it is this way

Memories are intentionally separate from the global event log. The log is a
colony-wide feed with a short display-oriented lifetime, while memory belongs to
one colonist and must survive unrelated colony events. Recording at completed
actions avoids filling the history with progress ticks, movement, or idle
behavior. A fixed cap keeps long simulations from growing every colonist
without bound; newest-first eviction preserves the useful recent context.

## Extending it

Add a call to `World.remember` at the semantic completion point of a new notable
action. Do not record state transitions such as `Moving` or `Idle`. If a new
kind of perception should be remembered, add it to `observeNearby` and keep
sightings edge-triggered.

## Related

- [entities-and-ai.md](./entities-and-ai.md)
- [architecture.md](./architecture.md)
