# Drives redesign (proposal)

> Part of the [mars-sim documentation](./README.md).

**Status: proposal.** Phase D0 (the rename and the `Consequence` enum) has
shipped; everything after it is a plan. [drives.md](./drives.md) describes
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
get depressed. Beauty is not even something a colonist goes and gets; it is
something its surroundings do to it.

| Drive | Accumulates from | Discharged by | Consequence at the ceiling |
| --- | --- | --- | --- |
| food | time | eating (meal, pod) | **death** (starvation) — *shipped* |
| bladder | time | a toilet | **soiling oneself** → embarrassment |
| social | time (trait-scaled) | conversation | **depression** |
| sleep | time | a bed | none yet |
| hygiene *(new)* | time, and dirty work | washing | open — see below |
| comfort *(new)* | standing, working, hard surfaces | sitting, a good bed | open |
| beauty *(new)* | ugly surroundings | pleasant surroundings | open |

## Source

Shipped so far (D0):

- [`internal/sim/drives.go`](../internal/sim/drives.go) — `DriveKind`,
  `DriveSpec`, `Consequence`, `applyDriveConsequences`, `starve`.
- Everything else is still planned; this section grows as phases land.

## How it works

### Three kinds of consequence

The useful systematization is not one consequence per drive, it is that the
consequences fall into three *shapes*, and each shape is one piece of code
that any drive can use:

| Shape | While at the ceiling… | Example | Reversible? |
| --- | --- | --- | --- |
| **Drain** | something is lost every tick | starvation: HP | yes: satisfying the drive heals what it drained (`starvationDamage`) |
| **Event** | something happens *once*, and discharges the drive | soiling oneself | no: it happened, and it is remembered |
| **Condition** | after enough time, the colonist *becomes* something, and stays that way after the drive is met | depression | slowly: the condition has its own recovery |

`Consequence` stays an enum (design principle 1: switch on a name, not a
number), but each constant maps onto one of these shapes, so adding the next
consequence of an existing shape is small:

```
ConsequenceNone
ConsequenceDeath      // drain: HP                     (shipped)
ConsequenceSoiling    // event: discharge + occurrence (D1)
ConsequenceDepression // condition: Depressed          (D2)
```

### Soiling (bladder, D1)

At the ceiling, the colonist wets itself where it stands:

1. The bladder drive resets to 0 (the event discharges it — a colonist is
   never stuck at a bladder ceiling, which today it can be indefinitely).
2. A puddle of refuse goes on the tile, which the existing sanitation system
   already knows how to clean ([sanitation.md](./sanitation.md)).
3. An occurrence goes through the perception grammar
   ([compositional-perception-and-events.md](./compositional-perception-and-events.md)):
   `actor: colonist, action: soil, object: self`. The actor's reaction is
   **embarrassment** (grip down, valence down) and a memory ("Wet themself.").
   Witnesses get their own reaction rows — that is where a trait like Tidy
   can be disgusted and a cruel one amused — and a relationship nudge if we
   want one. All of that is `cognition.yaml` rows, not code.
4. Later, once hygiene exists, it spikes the hygiene drive.

This is deliberately built from existing mechanics (principle 10): refuse,
perception rules, reactions, memories. The only new code is the event branch
in `applyDriveConsequences`.

Embarrassment should *not* be a new affect axis. It is a reaction whose target
lands in the low-grip, low-valence part of the plane; whether it reads as a
label in the UI is an attractor question in `cognition.yaml`.

### Depression (social, D2)

Depression is the first **condition**, and conditions are a new mechanic, so
they get the most design care:

- **A condition is an enum on the colonist** (`Depressed`), not a reading of
  valence. Principle 1 already calls out "every system reads charge/grip/valence
  itself" as the mistake; a condition is the named, behavioral state other
  systems switch on, with its thresholds in one place.
- **Onset needs time at the ceiling, not a touch.** A colonist who brushes
  `Max` once is lonely, not depressed. Track ticks spent at the ceiling
  (lazily, like the level) and set the condition past an onset duration.
- **Recovery is its own rule.** Meeting the drive stops the clock but does
  not cure; recovery takes sustained time with the drive low. This is what
  makes it a condition rather than a drain.
- **Effects** are the questions to settle with play: a lower `affectHome`
  (charge and valence), slower work (`workScale`), and less pull toward
  socializing. That last one is a death spiral — less socializing, more
  depression — so per principle 7 it needs an exit in the same change:
  friends seeking out a depressed colonist, or a floor on social pressure.

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

### Hygiene (D3)

Hygiene rises with time *plus* events: digging, cleaning gore, hauling
corpses, soiling oneself. Event-driven rises are bumps to the stored base, so
they also fit the lazy model. It is discharged at a new wash facility. Its
consequence is open (see the questions below).

### Which drive wins

`mostUrgentDrive` and focus eligibility currently ask "is it fatal?". With
four consequences that becomes a **severity** ordering on `Consequence`
(death above condition above event above none), so the rule "starving beats a
full bladder" generalizes without a special case. Within a severity, the
drive furthest past its threshold still wins.

## Why it is this way

- **Consequence is identity, not a setting.** The old `needs.food.fatal`
  knob let a settings file make bladder fatal, which silently rewrote the
  arbitration rules. Rise and thresholds are balance; what a drive *does* is
  code. D0 removed the knob for that reason.
- **Shapes, not bespoke consequences.** Writing `soil()`, `depress()`,
  `starve()` each as its own special case is the ad-hoc design this replaces.
  A drain, an event and a condition are each written once.
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
| D1 | `ConsequenceSoiling`: discharge, puddle, `soil` occurrence, embarrassment reaction | bladder ceiling now resolves |
| D2 | Conditions; `ConsequenceDepression` for social | new colonist state |
| D3 | Hygiene drive, wash facility, event bumps | new drive and facility |
| D4 | Room scores; beauty and comfort with environmental rise | new drives |
| D5 | Severity ordering replaces fatal/non-fatal; rename the remaining "need" vocabulary in `cognition.yaml` (`need_weight`, `fatal_bonus`, the `need` noun, `need-satisfied`) and the Scum Lab bench | arbitration generalizes |

D5's renames are deferred on purpose: `cognition.yaml` and the Scum Lab
share a file format with the browser tool, and renaming them alongside D0
would have doubled that diff for no behavior.

### Open questions

- **Hygiene's consequence.** Illness (a drain on HP? a condition?), or purely
  social — others react to a filthy colonist through perception?
- **Comfort and beauty consequences.** Probably conditions (aching, dreary)
  or just affect pressure with no ceiling consequence at all. Is "no
  consequence, only pressure" acceptable for a drive?
- **Do sleep's consequences exist?** Collapsing where you stand (an event) is
  the natural one.
- **Depression's exit.** Which way out do we want: friends seeking out the
  depressed, a treatment facility, or time alone?
- **Do rats and aliens get drives beyond food?** Today they share `DriveFood`
  only.

## Extending it

Until D1 lands, follow "Extending it" in [drives.md](./drives.md). After each
phase, the invariants to keep:

- Levels stay lazy. Anything that changes a rise rate re-bases first.
- Gameplay consequences use `World.rng`; flavor (who laughs at a soiled
  colonist) uses `World.prng` ([determinism.md](./determinism.md)).
- A consequence that can strand or kill a colonist ships with its exit.

## Related

- [drives.md](./drives.md) — the drives system as it exists today.
- [affect.md](./affect.md) — where embarrassment and depression's mood effects land.
- [compositional-perception-and-events.md](./compositional-perception-and-events.md) — the occurrence/reaction grammar soiling will use.
- [sanitation.md](./sanitation.md) — refuse, which soiling produces.
- [design-principles.md](./design-principles.md) — principles 1, 7, 9 and 10 shape most of this.
