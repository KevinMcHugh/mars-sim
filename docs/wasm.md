# WASM and the mind/sim split

> Part of the [mars-sim documentation](./README.md).

## What it is

The browser build of the sim, today used only by [Scum Lab](./scum-lab.md). It rolls one colonist and scores one focus. It does not boot a world, and it is not the future game client. A game frontend would be another consumer of snapshots and commands ([architecture.md](./architecture.md)); this module is the pure decisions that client and the bench both need.

There is no `internal/mind` package yet. The functions live in `internal/sim`. The split below is the cut that makes the module small. Do it by moving types, not by copying the functions.

## Source

- [`tools/scum-lab/wasm/main.go`](../tools/scum-lab/wasm/main.go) — `//go:build js && wasm`. Exports `catalog`, `roll`, and `evaluate` on `globalThis.scumlab`.
- [`tools/scum-lab/wasm/host.go`](../tools/scum-lab/wasm/host.go) — the non-wasm `main`, so `go build ./...` still succeeds.
- [`internal/sim/lab.go`](../internal/sim/lab.go) — `LabRoll`, `LabEvaluate`, `LabSettings`, `LabTraits`. The bench API.
- [`tools/scum-lab/shared/sim.js`](../tools/scum-lab/shared/sim.js) — fetches the module as bytes (`instantiate`, not `instantiateStreaming`) and sends the open file as JSON.
- [`tools/scum-lab/build.sh`](../tools/scum-lab/build.sh) — copies `wasm_exec.js` from `$(go env GOROOT)` and builds `tools/scum-lab/scumlab.wasm`.

`World.chooseFocus` calls `fillFocusCandidates` then `selectFocus`. `World.assignPersonality` calls `rollProfile`, then the colony-only name uniquify. `LabEvaluate` and `LabRoll` call those same functions. Stimulus bias is `accumulateStimulusBias`. Mood label is `moodReadout`. Need phase is `phaseForLevel`. Trait baseline is `clampedAffectHome`.

## How it works

`LabSettings` returns `defaultNeeds()` plus the trait-chance and mood constants. It does not call `DefaultConfig()`: that builds `DefaultCognitionConfig()`, and the bench already has the open file.

`LabEvaluate` decodes the file the page is editing (focus specs, arbitration, attractors, live stimuli) and the situation. It fills candidates, applies the bench reachability gate, then `selectFocus`. The gate is `applyBenchReach` in `lab.go`. It sets `Eligible` false and drops commitment on the current focus. It does not change the need score. Do not fold that gate into `chooseFocus`; the sim still lets a need win and then walks or builds.

Reasons under the bars are `explainFocus`. They describe the flags the scorer already set. They are not a second eligibility pass.

Build and open:

```sh
tools/scum-lab/build.sh
tools/scum-lab/build.sh --open
```

`wasm_exec.js` must match the Go that compiled the `.wasm`. The script copies it every build. Rebuild after a Go upgrade.

## Why it is this way

Importing `internal/sim` from the browser runs that package's initializer and every import it has. Two of those blow the module up:

- `cognition_config.go` imports `gopkg.in/yaml.v3`. The bench never parses YAML in Go (an overlay on `DefaultCognitionConfig` would resurrect a shipped rule the editor deleted), but the import alone runs yaml's init.
- A package-level `map[WearPolicyID]func(*World, …)` made `*World` reachable. The linker then kept every world method. Wear lookup is now `wearPolicy(id)`, a function. Do not put `*World` in the type of a package-level variable while this wasm imports `internal/sim`.

`newWorld` is not in the module. The module is still several megabytes, mostly the Go runtime plus yaml's init plus `encoding/json`.

The small build is a new package, `internal/mind`, that holds the focus and personality functions and the types they need. It must not import the YAML loader and must not mention `*World`. `internal/sim` imports it; world methods stay wrappers. The wasm `main` imports only `internal/mind`. Moving the types (`FocusKind`, `DriveSpec`, `Trait`, the roll tables) is the work. A second copy of `fillFocusCandidates` or `rollProfile` in JavaScript or in the wasm package is the failure mode.

`LabEvaluate` takes the editor's JSON, not `ApplyCognitionYAML`, on purpose. The loader overlays a document on the shipped defaults. The bench has to score the file on screen, including a deleted rule.

## Extending it

A new bench question that the sim already answers is a function of plain values, called from the world method and from `LabEvaluate` or a sibling. Wire it through `wasm/main.go` and `shared/sim.js`. Keep `host.go` so the package still builds on the host.

A new world fact the bench must show is a field on `focusInputs`, filled by `World.focusCandidates` from the entity and by `LabEvaluate` from the situation. One filler.

When the `mind` package exists, the wasm import path changes and this doc's "not yet" sentence goes away. Until then, do not start the package by pasting the functions.

## Related

- [scum-lab.md](./scum-lab.md) — the page that loads the module.
- [architecture.md](./architecture.md) — snapshots and commands, the contract a game frontend would use.
- [personality.md](./personality.md) — the roll, and why age has its own stream.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — what `fillFocusCandidates` is implementing.
