# Settings file

> Part of the [mars-sim documentation](./README.md).

## What it is

`mars-sim.yaml` is a committed set of options: the middle layer between the
defaults compiled into `DefaultConfig()` and the flags you type at the shell.
It ships fully commented out — every setting shown at its default value, with a
line of documentation above it — so the file as generated changes nothing. You
uncomment a line to change it, and commit the result so everyone (and CI, and
your next session) plays the same colony.

Not to be confused with the embedded data files,
[`internal/sim/events.yaml`](../internal/sim/events.yaml) and
[`internal/sim/traits.yaml`](../internal/sim/traits.yaml). Those are compiled
into the binary and declare what life events *mean* and how traits react to
them; this one is generated from `sim.Config` and holds the pacing a player is
meant to turn. A number belongs there if changing it changes what something is,
and here if it changes how fast or how much.

Settings apply in three layers, each overriding the one before it:

```
DefaultConfig()  →  mars-sim.yaml  →  command-line flags
```

## Source

- [`internal/sim/configfile.go`](../internal/sim/configfile.go) — `Knobs`,
  `ConfigTemplate`, `ApplyConfigFile`, `ConfigKeys`.
- [`internal/sim/config.go`](../internal/sim/config.go) — the `cfg`/`doc`/`sec`
  struct tags that define the knobs.
- [`main.go`](../main.go) — `configPathFromArgs`, `loadConfigFile`,
  `bindConfigFlags`.
- [`mars-sim.yaml`](../mars-sim.yaml) — the committed file itself.
- [`internal/sim/configfile_test.go`](../internal/sim/configfile_test.go) and
  [`config_test.go`](../config_test.go) — round-trip, layering, and the
  staleness check on the committed file.

## How it works

### One tag, three surfaces

Every tunable in `Config` carries struct tags:

```go
StartColonists int `cfg:"colonists" sec:"Starting population" doc:"starting number of colonists"`
```

- `cfg` is the name: the key in the settings file *and* the command-line flag.
- `doc` is the one-line description printed in `-h` and above the key in the file.
- `sec`, on the first field of a group, starts a section in the generated file.

`sim.Knobs(&cfg)` walks those tags by reflection and returns a `Knob` per
tunable, each holding a pointer into the `Config` you passed. `bindConfigFlags`
type-switches over those pointers to register flags; `ConfigTemplate` renders
them as commented YAML; `ApplyConfigFile` looks them up by key. Add a field with
a tag and all three surfaces get it.

`Config.Needs` and `Config.Focuses` are arrays of specs, so their knobs nest:
the file writes `needs.food.rise` and `focuses.work.base`, while flags flatten
those to `-need-food-rise` and `-focus-work-base`. Element names come from the
corresponding enum's `String()` method, so both surfaces read the way a player
would say them.

### Startup order

`main` reads the file *before* it registers any flag, so the file's values
become the flag defaults. Flags then override the file for free, and `-h`
prints the values this run will actually use. The cost is that `-config` has to
be found by hand (`configPathFromArgs`) before `flag.Parse` runs.

| Flag | Behavior |
| --- | --- |
| *(none)* | Read `mars-sim.yaml` from the working directory if it exists. |
| `-config PATH` | Read that file instead. A file named here that does not exist is an error. |
| `-config ""` | Read no file at all. |
| `-print-config` | Write a fresh commented template to stdout and exit. |

`seed` is in the file but is not an ordinary knob: like `-seed`, `0` means
"pick a fresh time-based seed", so a committed file can pin a world without
`Config.Seed`'s clock-based default leaking into a generated template.

### Rejected settings

A misspelled key, a value of the wrong type, and the same key set twice are all
errors that stop startup, quoting the file and line:

```
mars-sim: mars-sim.yaml:31: unknown setting "colonits" (run with -print-config to list every setting)
```

## Why it is this way

- **Committed, not gitignored.** The whole point is to check in a balance. A
  personal file is still easy — `-config mine.yaml`, or don't commit your edits.
- **All commented out** rather than a file of live defaults. An inert file can
  be regenerated, diffs show only what a player deliberately changed, and a
  default that moves in `config.go` still reaches anyone who never touched that
  line. The cost is that nested settings need their parent keys uncommented
  too, which the file says at the top of the needs section.
- **Unknown keys are fatal.** A typo in a committed settings file that silently
  does nothing is exactly the failure this file exists to prevent, and a game
  that quietly ignores your balance is worse than one that will not start.
- **Reflection over a hand-written list.** The flags used to be ~80 hand-written
  `flag.IntVar` lines. Adding the file would have made that two hand-written
  lists to keep in step, and `StuckLimit` had *already* drifted out of the flag
  list. Tagging the field is the single edit; the rest is derived.
- **Flag defaults come from the file**, rather than applying the file after
  parsing. Comparing "was this flag passed?" against `flag.Visit` works too, but
  it leaves `-h` advertising defaults the run will not use.
- **The template is generated, the committed file is not regenerated for you.**
  Regenerating would clobber the values a player set. Instead a test fails when
  the committed file is missing a setting, and says which command to run.

## Extending it

Adding a tunable is a two-step edit:

1. Add the field to `Config`, grouped with related fields, with `cfg` and `doc`
   tags (and `sec` if it starts a new group). Set its default in
   `DefaultConfig()`.
2. Regenerate the settings file: `go run . -print-config > mars-sim.yaml`,
   re-applying any values that were uncommented in it. Add a `validateConfig`
   case in `main.go` if bad values would break the sim.

`TestCommittedSettingsFileCoversEverySetting` fails until step 2 is done, and
`TestTemplateRoundTrip` fails if a knob does not survive a write-and-read.

Only `int`, `int64` and `bool` fields can be knobs. A new kind needs a case in
`assign` (configfile.go), `bindConfigFlags` (main.go), and `formatValue`.

## Related

- [configuration.md](./configuration.md) — the `Config` struct these settings fill in.
- [cli.md](./cli.md) — the flags that sit on top of the file.
- [needs.md](./needs.md) — the `NeedSpec` table that nests inside the file.
