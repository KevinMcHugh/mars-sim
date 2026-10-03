# Alien taxonomy (scientific names)

> Part of the [mars-sim documentation](./README.md).

## What it is

Every rolled alien species gets a scientific name as well as the colloquial
one colonists use: a binomial such as *Pseudursus ares*, *Erythrosaurus
ferox* or *Hemerocyclops placens*. It is built from Greek and Latin word
parts that fit the species' build, so the name is a coded description of the
animal, the way real names are: *Lepidoraptor* is a scaly hunter, *Hexapus*
has six legs. It is flavor only. The lore tab shows it, in the TUI and the
browser, in italics under the species' title.

## Source

- [`internal/sim/alien_taxonomy.go`](../internal/sim/alien_taxonomy.go):
  `alienTaxonomy`, `taxonEntry`, `loadTaxonomy`, `scientificName`,
  `joinTaxa` (vowel elision), and the `alienTaxonomySeed` stream key.
- [`internal/sim/alien-taxonomy.yaml`](../internal/sim/alien-taxonomy.yaml):
  the word parts (prefixes, roots, epithets), each with an optional `when`
  condition. Embedded with `go:embed`.
- [`internal/sim/lore.go`](../internal/sim/lore.go):
  `AlienSpecies.ScientificName`, filled in by `rollAlienSpeciesRoster`.
- [`internal/sim/alien_taxonomy_test.go`](../internal/sim/alien_taxonomy_test.go):
  well-formed, unique and reproducible names; vowel elision; parts that
  respect their conditions; and naming that leaves the rest of the roster
  roll unchanged.
- Display: `renderLoreDetail` in `internal/ui/tui/render_lore.go`,
  `LoreSpecies.ScientificName` in `internal/wire/topics.go`, and
  `web/src/ui/LorePanel.svelte`.

## How it works

A binomial is `Genus epithet`. The genus is capitalized and the epithet is
lowercase; both are italicized when displayed.

```
genus   = prefix + root     erythro + saurus  -> Erythrosaurus
epithet = epithet           ferox
```

`alien-taxonomy.yaml` has three lists: **prefixes** (combining forms such
as `erythro-` red, `lepido-` scaly, `deino-` terrible, `areo-` Mars), **roots** (the
noun the genus ends in, such as `-saurus` lizard, `-hexapus` six-footed, `-odon`
tooth, `-therium` beast) and **epithets** (`ferox` fierce, `martis` of Mars,
`gigas` giant). Each entry has a `when` that uses the same `nameCondition`
tree as [alien-names.yaml](./lore.md#naming-a-condition-gated-pool-not-a-flat-table)
(temperament, skin, color, pattern, height/weight tier, tail, wings, and counts of
legs, arms, limbs and eyes). `scientificName` draws in this order:

1. A **root**, from those whose condition the species meets.
2. A **prefix**, from those that match. A prefix marked `mimic: true`
   (`pseudo-`, "false") is a candidate only when the root is also
   `mimic: true`, meaning an Earth animal the species merely resembles
   (`ursus` bear, `saurus` lizard, `ophis` snake). That is how
   *Pseudursus* comes about, while *Pseudodon* ("false tooth") cannot.
3. `joinTaxa` glues the two together. When a prefix ending in a vowel meets
   a root starting with one, the prefix drops its vowel, as real names do:
   `erythro + ops` gives *Erythrops* and `pseudo + ursus` gives
   *Pseudursus*. `y` is not treated as a vowel, so `dasy + urus` gives
   *Dasyurus*, which is the real genus.
4. An **epithet**, from those that match.

Each list has unconditional entries (`areo`, `therium`/`morphus`/`zoon`,
and the Martian place genitives), so a name can always be built.
`loadTaxonomy` refuses a file that lacks one, and it also refuses any form
that isn't lowercase `a-z`.

No two species in a roster share a binomial. On a collision
`scientificName` draws again, up to 16 times. If every attempt collides,
which only a huge roster could cause, it numbers the name ("Areozoon
martis 2"), the same way `distinctAlienName` numbers a common name.

## Why it is this way

- **A separate RNG stream (`Seed ^ alienTaxonomySeed`), and names drawn
  after the whole roster.** The obvious approach was to draw the name from
  the lore stream inside `rollAlienSpecies`, but every draw there shifts
  every species rolled after it. Adding scientific names would then have
  re-rolled the build, temperament and bite damage of every existing seed's
  second species onward. With its own stream, the roster is
  byte-for-byte what it was before, and
  `TestScientificNamesDoNotShiftTheRoster` pins that. Because nothing
  else draws from the stream, re-drawing on a collision is free. (The common
  name can't do that; see lore.md's "Uniqueness filters the draw rather
  than re-rolling".) The stream is used only in `newWorld`, like the lore
  stream, so it needs no save state.
- **Epithets that never change form.** In Latin, an adjective epithet has
  to agree in gender with its genus: *Ursus ruber* but *Teuthis rubra* and
  *Zoon rubrum*. Getting that right means tracking the gender of every root,
  and the gender a compound takes isn't always obvious. Instead, every
  built-in epithet is one whose form doesn't depend on gender:
  - genitives: *martis* "of Mars", *utopiae*
  - nouns in apposition: *ares*, *gigas*, *comes*, *cavernicola*
  - third-declension adjectives whose nominative is the same in every
    gender: *ferox*, *velox*, *fallax*, *latens*, *versicolor*, *multipes*,
    and the present participles *tremens* (gelatinous), *horrens* (hairy),
    *volans* (winged)

  That ruled out the obvious hide words: *lapideus* "stony", *hirsutus*
  "hairy", *alatus* "winged" and *pennatus* "feathered" all decline by
  gender, so the rocky, woody and feathered hides use *lapis* and *arbor*
  (in apposition) and *plumipes* "feather-footed" instead. *volans* is kept
  even though no species flies yet: it names what the wings look like they
  are for, as *Draco volans* does for a lizard that only glides.

  Real taxonomists rely on the same forms all the time. Keep to them when
  you add an epithet, or add gender tracking first.
- **Data, not a Go switch,** for the same reasons as the colloquial name
  pool (see [lore.md](./lore.md)): adding a word part is a YAML edit, and
  the condition language is one contributors already know.
- **Roots are allowed to repeat a prefix's trait.** *Lepidosaurus* is
  "scaly lizard", which is redundant, but real names are often like that.
  Filtering it out would make scaly species rarer to name. The same goes
  for *Pteropteryx* ("wing-wing") on a winged species.
- **Embedded only, no `-alien-taxonomy` flag yet.** The common-name pool
  needed a runtime override because players wanted to edit it. Nobody has
  asked to edit the word parts, so the override was left out. Plumbing one
  through would follow `-alien-names` exactly.

## Extending it

- **More word parts:** add an entry to `alien-taxonomy.yaml` with whatever
  `when` fits. A form is lowercase ASCII. An epithet should be invariable
  (see above). Run `go test ./internal/sim -run Taxonomy` afterwards.
- **Eponyms and colony naming.** TODO.md wants colonists to name new species
  themselves. Scientific practice offers an obvious hook: an epithet
  honoring the discoverer, the Latinized genitive of their surname
  (*smithi*, *garciae*). That means naming the species when the colony first
  meets it, rather than at worldgen, which ties into the "fuller codex"
  idea in lore.md.
- **Gendered epithets** (*ruber/rubra/rubrum*) would need a `gender` on
  each root and three forms on each adjective.
- **Higher ranks** (a family *-idae* shared by species with the same root)
  could group a multi-species roster in the lore tab.

## Related

- [lore.md](./lore.md): species rolling, the common-name pool, and the
  `nameCondition` tree this reuses.
- [rng-streams.md](./rng-streams.md): the stream table this adds a row to.
- [determinism.md](./determinism.md): why a new draw must not land on an
  existing stream.
- [frontend-tui.md](./frontend-tui.md) and
  [frontend-web.md](./frontend-web.md): the lore tabs that show it.
