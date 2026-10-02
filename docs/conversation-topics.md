# Conversation topics

> Part of the [mars-sim documentation](./README.md).

## What it is

Every finished conversation is now *about* something. One of the two
colonists raises a topic it actually has: one of its own memories, another
colonist it has feelings about, or a piece of lore about the world (today an
alien species). Both remember what they talked about, and talking about a
third colonist is gossip: a good chat carries the speaker's opinion of that
colonist over to the listener.

## Source

- [`internal/sim/topics.go`](../internal/sim/topics.go) — `TopicKind`,
  `ConversationTopic`, the `LoreItem` interface and `LoreKind`,
  `speciesLore`, `World.loreItems`, `chooseTopic`, `recountableMemories`,
  `gossipSubjects`, `applyGossip`, `conversationText`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `finishTalk`,
  which chooses the topic, applies gossip, and writes the memory text.
- [`internal/sim/config.go`](../internal/sim/config.go) — the
  `talk-topic-*-weight` and `talk-gossip-percent` tunables.
- [`internal/sim/rng.go`](../internal/sim/rng.go),
  [`internal/sim/world.go`](../internal/sim/world.go) — `topicRNG`, the
  topic stream.
- [`internal/sim/topics_test.go`](../internal/sim/topics_test.go) — the
  tests that pin this behavior.

## How it works

`finishTalk` rolls quality and credits affinity exactly as before, then calls
`chooseTopic(a, b)`:

1. **Speaker.** One of the two is drawn to raise the topic; the other listens.
2. **Candidates.** For each kind, what the speaker has to say:

   | Kind | Candidates | Weight |
   | --- | --- | --- |
   | `TopicMemory` | the speaker's memories, minus its other conversations | `talk-topic-memory-weight` (3) |
   | `TopicColonist` | living colonists, not the two talking, the speaker has a nonzero affinity toward, in ID order | `talk-topic-colonist-weight` (3) |
   | `TopicLore` | `World.loreItems()` — every alien species | `talk-topic-lore-weight` (2) |

3. **Draw.** A kind is drawn by weight among the kinds with at least one
   candidate, then one candidate uniformly. If every enabled kind is empty
   the result is `TopicNone` — small talk, and the memory reads as it always
   did: "Had a conversation with Bo."

The topic shapes both participants' memory text:

| Topic | Speaker remembers | Listener remembers |
| --- | --- | --- |
| memory | `Had a conversation with Bo, recalling: "Had a meal."` | `Had a conversation with Ada, who recalled: "Had a meal."` |
| colonist | `Had a conversation with Bo about Cy.` | same, with the other name |
| lore | `Had a conversation with Bo about the grelks.` | same, with the other name |

**Gossip.** For a colonist topic, `applyGossip` moves the listener's affinity
toward the subject `talk-gossip-percent` (10%) of the way to the speaker's —
but only when the conversation's quality was positive. A bad chat convinces
nobody.

## Why it is this way

- **Speakers only raise what they have.** Kinds with no candidates drop out
  of the weighting rather than being drawn and then falling back. A newly
  landed colonist with no opinions of anyone talks about what it remembers or
  about the local wildlife; it does not burn a colonist-topic draw on
  nothing and land on small talk.
- **Gossip converges, it does not add.** An additive "speaker's opinion ×
  percent" would let a chain of gossip push a listener past the speaker's
  own view, and two colonists who already agree would keep reinforcing each
  other every chat. Moving the listener *toward* the speaker's value is
  bounded by it, and is a no-op between people who already agree. Integer
  division means small gaps (under 10 points at the default 10%) never
  close — the faint opinions are not worth passing on.
- **Its own RNG stream.** Gossip moves affinity, so topics are not flavor
  and cannot ride the personality stream (`prng`). Drawing them from the
  simulation stream (`rng`) would shift every later draw — every seed's game
  would change just because conversations gained topics. `topicRNG` follows
  the same reasoning as `skillRNG`; see [rng-streams.md](./rng-streams.md).
- **No map iteration.** Gossip subjects come from `entityIDsSorted`, not from
  ranging over the speaker's affinity map, so map order never picks who is
  talked about ([determinism.md](./determinism.md)).
- **Conversations are not retold.** A conversation memory's text already
  embeds a topic; retelling one nests quotes inside quotes. They are excluded
  from memory candidates.
- **Every species is talkable, met or not.** There is no per-colonist (or
  per-colony) knowledge of lore yet to gate on, so colonists talk about any
  rolled species as if they arrived knowing the survey. Gating lore on
  sightings belongs with the codex work in [lore.md](./lore.md).

## Extending it

- **A new kind of lore (history, organizations, other colonies).** Implement
  `LoreItem` — `LoreKind()` (add a `LoreKind` constant), a world-unique
  `LoreID()`, and `TopicPhrase()` (the object of "about") — and append the
  items in `World.loreItems`. Keep that order stable: topic draws index into
  it. Nothing in the conversation code needs to change.
- **Effects for memory and lore topics.** The chosen `ConversationTopic`
  carries the full `Memory` copy or the `LoreItem`, so an effect (e.g.
  hearing about a hostile species nudging the listener's affect, or a shared
  memory of a dead friend) is a new function called next to `applyGossip` in
  `finishTalk`.
- **Who knows what.** If lore gains per-colonist knowledge, filter in
  `chooseTopic` (the speaker's candidates) and spread it in `finishTalk` —
  conversations are the natural way for knowledge to travel.
- Topics reach every frontend through the memory text, so the memories tab
  (TUI) and inspect panel (browser) show them already. There is no
  structured topic field on `Memory` or the wire; add one only once a
  frontend needs to filter on topic kind or link to the subject.

## Related

- [memories.md](./memories.md) — the conversation occurrence and memory
  funnel the topic text rides on.
- [lore.md](./lore.md) — the alien species that are today's only lore.
- [rng-streams.md](./rng-streams.md) — the topic stream.
- [meeting-hall.md](./meeting-hall.md) — where most conversations happen.
