# Drives redesign (proposal)

> Part of the [mars-sim documentation](./README.md).

**Status: proposal.** Phase D0 (the rename and the `Consequence` enum), D1a
(soiling), D2 (loneliness), D2b (passing out) and D3 (hygiene) have shipped;
everything else is a plan. [drives.md](./drives.md) describes
what the code does today. Update this doc as phases land, and move what is
built into drives.md.

## What it is

Needs were four hand-tuned special cases that happened to share a table. This
redesign makes them one system, **drives**, where every drive is described by
the same three things:

- **Accumulation** — how fast the level rises, and what makes it rise.
- **Thresholds** — when the colonist starts caring (`SeekAt`), when it becomes
  urgent (`CriticalAt`), and the ceiling (`Max`).
- **Consequence** — what happens to a colonist who reaches the ceiling.

"Drive" rather than "need" because not everything this models is a need. Food
is a need: go without it and you die. Company is not: go without it and you
feel lonely. Beauty is not even something a colonist goes and gets; it is
something its surroundings do to it.

| Drive | Accumulates from | Discharged by | Consequence at the ceiling |
| --- | --- | --- | --- |
| food | time | eating (meal, pod) | **death** (starvation) — *shipped* |
| bladder | time | a toilet | **soiling oneself** → embarrassment |
| social | time (trait-scaled) | conversation | **feeling lonely** — *shipped* |
| sleep | time | a bed | **passing out** — *shipped*; later, hallucinations from long-term deprivation |
| hygiene | time, and dirty work (grime) | a shower | **feeling filthy** — *shipped*; others' reactions later |
| comfort *(new)* | standing, working, hard surfaces | sitting, a good bed | open |
| beauty *(new)* | ugly surroundings | pleasant surroundings | open |

## Source

Shipped so far (D0, D1a, D2, D2b, D3):

- [`internal/sim/drives.go`](../internal/sim/drives.go) — `DriveKind`,
  `DriveSpec`, `Consequence`, `applyDriveConsequences`, `starve`,
  `consequenceDue`, `feel`, `addGrime`, `passOut`, `stayPassedOut`, `wetSelf`, `usingFacility`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `finishTalk`'s
  `socialize` occurrence.
- [`cognition.yaml`](../cognition.yaml) — the `felt-lonely`, `socialized`,
  `passed-out`, `soiled-self` and `witnessed-soiling` reactions, the
  `witness-soiling` perception rule, the `washed` and `felt-filthy`
  reactions, the `wash` focus, and the Tidy rules for soiling and filth.
- The `Shower` terrain and the planner's washroom
  ([`internal/sim/project.go`](../internal/sim/project.go)).
- Everything else is still planned; this section grows as phases land.

## How it works

### Three kinds of consequence

The useful systematization is not one consequence per drive, it is that the
consequences fall into three *shapes*, and each shape is one piece of code
that any drive can use:

| Shape | At the ceiling… | Example | Discharges the drive? |
| --- | --- | --- | --- |
| **Drain** | something is lost every tick; satisfying the drive gives it back | starvation: HP (`starvationDamage`) | no |
| **Experience** | the colonist *feels* something, an occurrence with a mood hit and a memory, repeated on a cadence while it stays there | feeling lonely | no |
| **Event** | something *happens*, once, and resets the drive | passing out; soiling oneself | yes |

`Consequence` stays an enum (design principle 1: switch on a name, not a
number), but each constant maps onto one of these shapes, so adding the next
consequence of an existing shape is small:

```
ConsequenceNone
ConsequenceDeath      // drain: HP                        (shipped)
ConsequenceLoneliness // experience: "felt lonely"        (shipped, D2)
ConsequencePassOut    // event: collapse, then discharge  (shipped, D2b)
ConsequenceSoiling    // event: discharge + occurrence    (shipped, D1a)
ConsequenceFilth      // experience: "felt filthy"        (shipped, D3)
```

An experience and an event both go through the perception grammar, so what
they *feel* like is `cognition.yaml`, not Go. The difference is that an event
changes the world (the drive resets, a puddle appears) and an experience only
changes the colonist.

### Soiling (bladder, D1a shipped; D1b the puddle)

At the ceiling, the colonist wets itself where it stands:

1. **Shipped (D1a).** The bladder drive resets to 0: the event discharges
   it, so a colonist is never stuck at a bladder ceiling again.
2. **Shipped (D1a).** A `soil` occurrence goes through the perception
   grammar. The colonist's reaction is embarrassment (`soiled-self`, a
   flush of charge, grip and valence down, "Wet herself." in the colonist's own
   pronouns); anyone within
   `gore-sight-radius` gets `witnessed-soiling`. Tidy colonists take both
   harder. All of that is `cognition.yaml` rows; the only new code is
   `wetSelf` and the `ConsequenceSoiling` branch.
3. **D1b.** A puddle on the tile that the sanitation system cleans
   ([sanitation.md](./sanitation.md)). This turned out not to be a small
   step, which is why it was split off: refuse today is gore and bodies,
   and it reaches frontends through its own section of the binary frame,
   so a new kind of refuse is a wire `Version` bump, both decoders, the
   golden frames, a TUI glyph and a browser tint, plus a cleaning path for
   refuse that is mopped up rather than carried to the incinerator. Rats
   eat gore (`scavenge.go`), so making a puddle "a kind of gore" would have
   been wrong as well as ugly.
4. **Later, with hygiene.** Soiling spikes the hygiene drive.

What D1a left out on purpose: a relationship cost for witnesses, and
amusement. There is no cruel trait to be amused; when there is, it is a
trait rule with negative scales, the way Mutant-Lover inverts a mutation.

Embarrassment is *not* a new affect axis. It is a reaction whose target
lands in the low-grip, low-valence part of the plane; whether it reads as a
label in the UI is an attractor question in `cognition.yaml`.

### Loneliness (social, D2 — shipped)

We considered making depression a **condition**: a named state on the
colonist with an onset, a recovery and its own effects. For now it is not.
Depression is just an experience, **felt lonely**, and it makes the colonist
feel worse; there is no new colonist state.

- At the ceiling the colonist feels lonely at once and again every
  `drives.social.consequence-every` ticks (200) while it stays there. The
  `felt-lonely` reaction pulls charge, grip and valence down, and its worn
  reading is worse than its fresh one. A colonist who keeps being lonely
  sinks further, and recovers as those memories roll off the bounded log.
  That compounding is the stand-in for depression, built entirely from
  wear (see [affect.md](./affect.md)).
- The cure is company. A conversation a colonist came to with its social drive
  at `SeekAt` or above is also experienced as **socialized**, which lifts
  charge, grip and valence on top of the conversation's own appraisal.

What a condition would add later is persistence that outlasts the memories,
and effects beyond mood (slower work, withdrawal). If we ever do that,
remember the warning that came with it: withdrawal feeds loneliness, so it
needs an exit (principle 7) in the same change.

### Environmental accumulation (beauty, comfort, D4)

Today every drive rises at a constant per-colonist rate, which is what makes
lazy evaluation exact: `level = base + rise × (now − since)`. Beauty and
comfort rise at a rate set by *where the colonist is*.

The lazy model survives this if the rate is piecewise constant: whenever the
rate changes (the colonist changes rooms, or the room it is in changes), fold
the level into the base and restart the clock with the new rate. That is the
same re-basing `resetDrive` already does. The cost rule (principle 9) is that
a room's beauty is computed when the room changes, not per tile per tick —
rooms already exist ([pathfinding.md](./pathfinding.md)), so this is a cached
score per room, invalidated by building, gore, refuse and corpses.

A drive's rise can also go **negative** in a pleasant room: beauty is
discharged by being somewhere nice rather than by using something. That means
not every drive has a facility or a focus. `DriveSpec` will grow a
"discharged by" kind (facility, item, conversation, environment) instead of
the `Facility: Rock` placeholder social uses today.

### Passing out (sleep, D2b — shipped)

At the sleep ceiling the colonist collapses where it stands and lies there
for `pass-out-ticks` (60, half again a night in a bunk), then comes to with
the drive met. It is an **event**: it happens once and discharges the drive.
The `passed-out` reaction hits grip hardest (losing control of your own
body) and is worse each time. While down, the colonist perceives nothing and
chooses nothing: it cannot flee, and an alien that finds it finds it
helpless. That is a death that comes from the story (principle 7), not a bug:
the colonist went 300 ticks past wanting a bed. The one exemption is a
colonist already asleep in its bed, whose drive can tick up to the ceiling
before the sleep finishes. The details are in [drives.md](./drives.md).

### Hallucinations (sleep, later)

Long-term sleep deprivation should eventually cause hallucinations. That
needs more redesign than any consequence above, for two reasons:

- **"Long-term" is not a level.** A drive's level resets every time it is met,
  and passing out meets it. Chronic deprivation needs a slower accumulator
  that outlives one ceiling: a sleep debt that grows with each pass-out and
  every short night, and decays only with real rest. That is either a second
  drive fed by the first, or a per-drive history the spec can read.
- **Perception only reports what happened.** Every percept today comes from a
  real occurrence. A hallucination is a percept with no occurrence behind it,
  seen by one colonist: an alien that isn't there, a voice. That means a new
  source of percepts (an "imagined" channel) that reactions can match, and a
  decision about which systems may act on it. Fleeing from a phantom alien is
  the point; the combat system shooting at one is a bug.

### Hygiene (D3 — shipped; D3b others' reactions)

Hygiene rises with time *plus* grime: digging, cleaning up gore and bodies,
and wetting oneself each add to it (`grime-mine`, `grime-clean`,
`grime-soil`). A grime bump re-bases the stored level, so it fits the lazy
model. It is discharged at a **shower**, which the planner builds in a
washroom after bunks and the trash room.

Its consequence comes in two steps:

1. **D3 (shipped): the colonist feels filthy.** An **experience**, the same
   shape as loneliness: a `felt-filthy` occurrence at the ceiling, repeated
   every `consequence-every` ticks (250), that makes the colonist feel worse.
2. **D3b: others react.** A filthy colonist is something *other* colonists
   perceive: a sight perception rule on a persistent "filthy" state, with
   reactions (Tidy colonists disgusted) and perhaps an affinity cost. That is
   a social consequence, so it waits for its own phase.

Tidy colonists' hygiene rises 1.5x, so they want a wash sooner, on top of
feeling soiling and filth twice as hard.

### Which drive wins

`mostUrgentDrive` and focus eligibility currently ask "is it fatal?". With
several consequences that becomes a **severity** ordering on `Consequence`
(death above event above experience above none), so the rule "starving beats a
full bladder" generalizes without a special case. Within a severity, the
drive furthest past its threshold still wins.

## Why it is this way

- **Consequence is identity, not a setting.** The old `needs.food.fatal`
  knob let a settings file make bladder fatal, which silently rewrote the
  arbitration rules. Rise and thresholds are balance; what a drive *does* is
  code. D0 removed the knob for that reason.
- **Shapes, not bespoke consequences.** Writing `soil()`, `depress()`,
  `starve()` each as its own special case is the ad-hoc design this replaces.
  A drain, an experience and an event are each written once.
- **Feelings are reactions, not code.** Loneliness could have been a direct
  nudge to affect. Going through an occurrence instead gives it wear, a
  memory, trait rules and a row anyone can tune in `cognition.yaml` or Scum
  Lab, for free.
- **Depression is not a state (yet).** A condition is a new mechanic with
  onset, recovery and a death-spiral risk. An experience that compounds through
  wear gets most of the feel with none of that.
- **Events discharge.** Without that, a colonist with no reachable toilet can
  sit at the bladder ceiling with nothing happening — the "nothing at the
  ceiling" case is the biggest gap in today's model.
- **The rename was done first, alone.** It is a large mechanical diff (every
  identifier, the settings keys, the browser wire topic). Doing it with no
  behavior change keeps every golden and determinism test identical, so the
  behavior changes in D1+ review on their own.

## Plan

| Phase | What | Gameplay change |
| --- | --- | --- |
| **D0** ✅ | Rename needs → drives everywhere (code, `drives.*` settings, `-drive-*` flags, TUI/web labels, wire API 8); `Consequence` enum replaces `Fatal`; `applyDriveConsequences` dispatches | none |
| **D1a** ✅ | `ConsequenceSoiling`: discharge, `soil` occurrence, embarrassment and witness reactions | bladder ceiling now resolves |
| D1b | The puddle: a mopped-up kind of refuse, on the wire and in both renderers | colonists clean up after it |
| **D2** ✅ | `ConsequenceLoneliness` (an experience, repeated every `consequence-every` ticks) for social; `socialized` for conversations sought while lonely | lonely colonists feel worse; company sought lifts mood |
| **D2b** ✅ | `ConsequencePassOut` for sleep: collapse for `pass-out-ticks`, `PassedOut` state, `passed-out` reaction | sleepless colonists drop where they stand |
| **D3** ✅ | Hygiene drive, shower and washroom, grime bumps; `ConsequenceFilth` ("felt filthy") | new drive and facility |
| D3b | Others perceive and react to a filthy colonist | social cost of filth |
| D4 | Room scores; beauty and comfort with environmental rise | new drives |
| D5 | Severity ordering replaces fatal/non-fatal; rename the remaining "need" vocabulary in `cognition.yaml` (`need_weight`, `fatal_bonus`, the `need` noun, `need-satisfied`) and the Scum Lab bench | arbitration generalizes |
| D6 | Sleep debt and hallucinations (imagined percepts) | needs the perception redesign above |

D5's renames are deferred on purpose: `cognition.yaml` and the Scum Lab
share a file format with the browser tool, and renaming them alongside D0
would have doubled that diff for no behavior.

### Open questions

- **Comfort and beauty consequences.** Probably experiences (aching, dreary)
  or just affect pressure with no ceiling consequence at all. Is "no
  consequence, only pressure" acceptable for a drive?
- **Should being attacked wake a passed-out colonist?** Today nothing does.
- **Is an experience enough for depression?** Revisit after play: if lonely
  colonists bounce back too fast, that is the case for a condition.
- **Do rats and aliens get drives beyond food?** Today they share `DriveFood`
  only.

## Extending it

Follow "Extending it" in [drives.md](./drives.md). After each
phase, the invariants to keep:

- Levels stay lazy. Anything that changes a rise rate re-bases first.
- Gameplay consequences use `World.rng`; flavor (who laughs at a soiled
  colonist) uses `World.prng` ([determinism.md](./determinism.md)).
- A consequence that can strand or kill a colonist ships with its exit.

## Related

- [drives.md](./drives.md) — the drives system as it exists today.
- [affect.md](./affect.md) — where embarrassment and loneliness land, and the wear that makes loneliness compound.
- [compositional-perception-and-events.md](./compositional-perception-and-events.md) — the occurrence/reaction grammar soiling will use.
- [sanitation.md](./sanitation.md) — refuse, which soiling produces.
- [design-principles.md](./design-principles.md) — principles 1, 7, 9 and 10 shape most of this.
