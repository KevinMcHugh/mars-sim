# Recruiting

> Part of the [mars-sim documentation](./README.md).

## What it is

The player growing the colony by hiring from off-world. The treasury pays a
recruiter for a set of candidates. Each one is a card: name, pronouns, age,
body, skills, traits, the emoji it will be drawn with, and the **savings** it
would bring. The player hires any of them, all of them, or none, or pays the
recruiter again for a new set. Each hire costs passage, and the recruit
appears at once beside one of the colony's ships with its savings in its
wallet and a few meals in its pockets.

It is the first thing the colony buys from outside the colony, so it is also
the first **sink** in the [money supply](./money.md): the fee and the passage
leave the game. A recruit's savings join it, so a lucky hire can bring in more
than it cost.

## Source

- [`internal/sim/recruit.go`](../internal/sim/recruit.go): the
  `RollRecruits` and `HireRecruits` commands, `rollCandidate`,
  `rollSavings`, `hireRecruits`, `arriveRecruit`, `recruitLanding`, and the
  snapshot's `RecruitingView`.
- [`internal/sim/money.go`](../internal/sim/money.go): `export`, the funnel
  money leaves the colony through, and `moneyExported`.
- [`internal/sim/world.go`](../internal/sim/world.go): `spawnWith`, which
  spawns a colonist from a candidate instead of rolling a new one, and the
  `recruits`, `recruitSets` and `recruitsHired` fields.
- [`internal/sim/personality.go`](../internal/sim/personality.go):
  `settlePersonality`, the half of `assignPersonality` a recruit still needs.
- [`internal/sim/skills.go`](../internal/sim/skills.go):
  `rollBackgroundFrom`, a background drawn from a given stream.
- [`internal/sim/config.go`](../internal/sim/config.go): the `recruit*`
  settings (the Recruiting section).
- [`internal/wire/recruit.go`](../internal/wire/recruit.go): the `recruit`
  topic.
- [`cmd/mars-sim-wasm/main.go`](../cmd/mars-sim-wasm/main.go): the
  `recruit-roll` and `recruit-hire` page commands (host API 16).
- [`web/src/ui/RecruitPanel.svelte`](../web/src/ui/RecruitPanel.svelte): the
  Recruit tab; `rollRecruits` and `hireRecruits` in
  [`web/src/game.svelte.ts`](../web/src/game.svelte.ts).
- [`internal/sim/recruit_test.go`](../internal/sim/recruit_test.go) and
  [`internal/wire/recruit_test.go`](../internal/wire/recruit_test.go).

## How it works

**Rolling.** `RollRecruits` pays `recruiter-fee` from the treasury with
`export` and replaces whatever set was on offer with `recruit-candidates`
new ones. It is refused, with a log line, if the treasury cannot pay or
recruiting is off (`recruit-candidates: 0`). Sets are numbered from 1.

**A candidate** (`rollCandidate`) is drawn whole, in advance, from the
recruit stream (`World.recruitRNG`):

- a profile by `rollProfile`, the same draw every colonist's personality is
  (age, gender, orientation, body, skin, hair, traits, name);
- a name nobody in the colony and nobody else in the set goes by;
- a background by `rollBackgroundFrom` (see [skills.md](./skills.md)),
  rolled onto a scratch `Entity` that holds nothing but the profile and the
  practice;
- savings by `rollSavings` (below).

**Hiring.** `HireRecruits{Offer, Picks}` names the set and the indices to
hire. A set that is no longer on offer is ignored, so a hire sent just before
a re-roll cannot hire from the new set. Repeated and out-of-range picks are
dropped. No picks turns the set away. Otherwise the whole hire is refused,
and the set stays on offer, if:

- the treasury cannot pay `recruit-cost` for every pick;
- the founders' ships are still aloft (see [ships.md](./ships.md)): there is
  no ship to arrive beside;
- `recruitLanding` cannot find a tile for everyone.

Then the passage is exported, and each pick arrives (`arriveRecruit`):
`spawnWith` makes the colonist from its candidate, the background's practice
and profession are copied on, the former employer is rolled as for anyone
(it is a pure function of the ID, so it could not be shown on the card), and
`recruit-meals` meals go in its pockets. The set closes and the log names
everyone who arrived and what they brought.

**What `spawnWith` does differently for a recruit:**

| Step | Founder or ship arrival | Recruit |
| --- | --- | --- |
| Profile | rolled on `prng` / `agePRNG` | the candidate's |
| Unique name, trait effects, mood | `settlePersonality` | `settlePersonality` |
| Family | `assignKin`, maybe a tie and a shared surname | its own tree node, no tie |
| Money | `crash-pod-purse`, minted | its savings, minted |
| Background | `rollBackground` on `skillRNG` | the candidate's |
| Meals | `crash-pod-meals` in its locker | `recruit-meals` in its pockets |
| Rare item | a gun, a chicken or a cat | none |
| Ship | the one it landed in, with a locker | none |

**Where they arrive** (`recruitLanding`). One of the colony's ships is picked
on the recruit stream. A breadth-first walk goes out from the ship's aisle
over walkable ground, and the recruits take the first plain floor tiles that
nobody stands on and that are not a doorway approach. So they come out in a
cluster, inside the ship if its aisle is clear and round it if not. If none
of the aisle is walkable (the ship was cleared and built over), the walk
starts from the nearest walkable tile to the ship's corner. The walk stops
after 4,096 tiles.

### Savings

`rollSavings` draws from a **log-normal** distribution with mean
`recruit-savings-mean` and standard deviation `recruit-savings-spread / 1.96`,
rounds to whole dollars, and clamps to `[recruit-savings-min,
recruit-savings-max]` (never below 0: nobody arrives in debt).

| Tuning | What you get |
| --- | --- |
| mean 100, spread 50 (the default) | about 95% of candidates bring $50–$150. A $500 set and five hires at $100 cost $1,000 and bring about $500, so recruiting shrinks the supply. |
| mean 1000, spread 10000 | about four in five bring less than $1,000, and about one in seventy $10,000 or more |
| spread 0 | everyone brings the mean |

`TestRecruitSavingsFollowTheirTuning` pins both tunings and the clamp.

### Money

`export(from, amount)` takes money out of an account and adds it to
`moneyExported`. It is `mint`'s mirror, and the audit grows a term:

```
treasury + Σ living wallets + moneyFrozen + moneyEscrowed + moneyExported
    == moneyIssued
```

`EconomyView.Exported` carries it, and both market tabs show it as
**Off-world** once it is above zero.

### The Recruit tab

`RecruitPanel.svelte` reads the `recruit` topic: the terms (fee, passage,
set size, meals), the treasury, the set on offer and its cards, whether the
founders have landed, and how many sets and recruits there have been. The
cards sit in a grid that fits as many across as the panel has room for: two
in the docked panel, all five in a popped-out window wide enough (see
[floating-panels.md](./floating-panels.md)). A ticked card is highlighted;
below the cards are the passage for the ticked ones and what they bring, and
**Hire**, **New set** and **Turn away**. The tab greys out what the engine
would refuse and says why; the engine checks again, and its log line is the
answer.

## Why it is this way

- **A stream of its own.** Rolling is player-driven and can happen any number
  of times. Drawn from `prng`, every re-roll would change who the next
  director arrival is; drawn from `rng`, it would shift every later gameplay
  draw. On `recruitRNG` it shifts nothing else:
  `TestRollingRecruitsShiftsNoOtherStream` lands a ship in two worlds, one of
  which rolled three sets first, and checks the colonist is the same. The
  stream is saved with the others (see [rng-streams.md](./rng-streams.md)).
- **Candidates are rolled whole, before they exist.** The card has to show
  who will arrive, so the profile, skills and savings are fixed at the roll.
  Rolling them again on hire would show one person and deliver another.
  `TestRecruitsArriveAsTheirCardsShowed` holds the card to the colonist.
- **No family.** A family tie renames the colonist (`inheritFamily` gives it
  the relative's surname) and changes its looks, so the card would be wrong.
  A recruit gets its own kin node, so later arrivals can still be related to
  it.
- **No ship, no locker, meals in the pockets.** The issue asked for recruits
  that simply appear. Stamping a ship of one per hire would cover the colony
  in hulls. Pockets are where `tryStartEating` looks first, so the meals are
  eaten before the colonist goes looking for a depot.
- **No rare item.** The gun, chicken or cat is part of the ship's manifest,
  and the trough a chicken needs is in the hold.
- **The fee and passage leave the game.** Paid to someone in the colony, they
  would be a transfer and change nothing. The point of recruiting from
  off-world is that the colony is buying something from outside it, which is
  the one thing that should cost the supply. Until there is export or trade
  between colonies, it is the only way money leaves.
- **Log-normal, not normal.** A normal distribution matches "95% within $50
  of $100", but with a wide spread it piles candidates up at the $0 clamp
  and still never produces a rich one. A log-normal with the same mean and
  standard deviation is almost normal when the spread is small and grows a
  long right tail when it is large. That is the "very unlikely rich colonist"
  the issue describes, and it never goes negative. One setting covers both
  tunings.
- **The spread is a 95% half-width, not a standard deviation.** That is how
  the issue asked to tune it. It holds while the spread is well under the
  mean; past that the tail is the point and the 95% band is lopsided.
- **The hire names its set.** Same reason `LandShip` names its ship: the
  topic lags a command, and a hire sent just after a re-roll would otherwise
  hire whoever happened to be at those indices in the new set.
- **All or nothing.** A hire the treasury can only partly pay for is refused
  whole, like an excavation (see [excavation.md](./excavation.md)), rather
  than hiring whoever the money reaches first.
- **A state proxy cannot cross to the worker.** The tab keeps its picks in
  Svelte `$state`, which is a proxy, and `postMessage` cannot clone one.
  `hireRecruits` copies the array before sending. The first version sent the
  proxy and the hire failed with a `DataCloneError` in the console.

## Extending it

- **The TUI**: the commands are frontend-neutral. A form in the TUI sends
  the same `RollRecruits` and `HireRecruits` and reads
  `Snapshot.Recruiting`.
- **Arriving by ship**: give `hireRecruits` a `landAt` path that stamps one
  ship for the whole hire. `land` would need to take candidates instead of
  rolling passengers.
- **A recruiter that knows the colony**: weight `rollCandidate`'s background
  toward the skills the colony lacks. Draw it from the recruit stream.
- **A new card field**: it goes on `recruitCandidate`, `CandidateView`, the
  wire's `Candidate` and the card. If `spawnWith` would overwrite it, copy it
  on in `arriveRecruit`.
- **Another off-world purchase** (a supply drop, a machine): pay with
  `export`, never by writing the treasury, or the money audit breaks.
- **Saving a game** needs `World.recruits`, `recruitSets`, `recruitsHired`
  and `moneyExported` saved with the rest.

## Related

- [money.md](./money.md): the supply, `mint`, and the audit `export` joins.
- [ships.md](./ships.md): the other way colonists arrive.
- [personality.md](./personality.md) and [skills.md](./skills.md): what a
  card shows.
- [rng-streams.md](./rng-streams.md): the recruit stream.
- [frontend-web.md](./frontend-web.md): the Recruit tab.
- [wire-format.md](./wire-format.md): the `recruit` topic.
- [colonist-looks.md](./colonist-looks.md): the emoji on each card.
