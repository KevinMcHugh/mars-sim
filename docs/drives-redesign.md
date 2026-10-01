# Drives redesign (proposal)

> Part of the [mars-sim documentation](./README.md).

**Status: proposal.** Phase D0 (the rename and the `Consequence` enum) and
D2 (loneliness) have shipped; everything else is a plan. [drives.md](./drives.md) describes
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
| sleep | time | a bed | none yet |
| hygiene *(new)* | time, and dirty work | washing | open — see below |
| comfort *(new)* | standing, working, hard surfaces | sitting, a good bed | open |
| beauty *(new)* | ugly surroundings | pleasant surroundings | open |

## Source

Shipped so far (D0, D2):

- [`internal/sim/drives.go`](../internal/sim/drives.go) — `DriveKind`,
  `DriveSpec`, `Consequence`, `applyDriveConsequences`, `starve`,
  `consequenceDue`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `finishTalk`'s
  `socialize` occurrence.
- [`cognition.yaml`](../cognition.yaml) — the `felt-lonely` and `socialized`
  reactions.
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
| **Event** | something *happens*, once, and resets the drive | soiling oneself | yes |

`Consequence` stays an enum (design principle 1: switch on a name, not a
number), but each constant maps onto one of these shapes, so adding the next
consequence of an existing shape is small:

```
ConsequenceNone
ConsequenceDeath      // drain: HP                        (shipped)
ConsequenceLoneliness // experience: "felt lonely"        (shipped, D2)
ConsequenceSoiling    // event: discharge + occurrence    (D1)
```

An experience and an event both go through the perception grammar, so what
they *feel* like is `cognition.yaml`, not Go. The difference is that an event
changes the world (the drive resets, a puddle appears) and an experience only
changes the colonist.

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

### Hygiene (D3)

Hygiene rises with time *plus* events: digging, cleaning gore, hauling
corpses, soiling oneself. Event-driven rises are bumps to the stored base, so
they also fit the lazy model. It is discharged at a new wash facility. Its
consequence is open (see the questions below).

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
| D1 | `ConsequenceSoiling`: discharge, puddle, `soil` occurrence, embarrassment reaction | bladder ceiling now resolves |
| **D2** ✅ | `ConsequenceLoneliness` (an experience, repeated every `consequence-every` ticks) for social; `socialized` for conversations sought while lonely | lonely colonists feel worse; company sought lifts mood |
| D3 | Hygiene drive, wash facility, event bumps | new drive and facility |
| D4 | Room scores; beauty and comfort with environmental rise | new drives |
| D5 | Severity ordering replaces fatal/non-fatal; rename the remaining "need" vocabulary in `cognition.yaml` (`need_weight`, `fatal_bonus`, the `need` noun, `need-satisfied`) and the Scum Lab bench | arbitration generalizes |

D5's renames are deferred on purpose: `cognition.yaml` and the Scum Lab
share a file format with the browser tool, and renaming them alongside D0
would have doubled that diff for no behavior.

### Open questions

- **Hygiene's consequence.** Illness (a drain on HP? a condition?), or purely
  social — others react to a filthy colonist through perception?
- **Comfort and beauty consequences.** Probably experiences (aching, dreary)
  or just affect pressure with no ceiling consequence at all. Is "no
  consequence, only pressure" acceptable for a drive?
- **Do sleep's consequences exist?** Collapsing where you stand (an event) is
  the natural one.
- **Is an experience enough for depression?** Revisit after play: if lonely
  colonists bounce back too fast, that is the case for a condition.
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
- [affect.md](./affect.md) — where embarrassment and loneliness land, and the wear that makes loneliness compound.
- [compositional-perception-and-events.md](./compositional-perception-and-events.md) — the occurrence/reaction grammar soiling will use.
- [sanitation.md](./sanitation.md) — refuse, which soiling produces.
- [design-principles.md](./design-principles.md) — principles 1, 7, 9 and 10 shape most of this.
