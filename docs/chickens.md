# Chickens

> Part of the [mars-sim documentation](./README.md).

## What it is

Every colonist lands with exactly one **rare item**: a gun, a chicken, or a
cat (see [crash-pods.md](./crash-pods.md)). A **chicken** (🐔) steps out of its
keeper's crash pod beside a **trough** (🪣) of **feed**. It eats feed from the
trough, or grazes cave scum off the rock when the trough is dry, and starves
with neither. Its keeper keeps the trough full: it scrapes scum, mixes it into
feed at a scumhouse, and tips the feed into the trough. Cats and chickens
ignore each other.

## Source

- [`internal/sim/chickens.go`](../internal/sim/chickens.go) — `feedRecipe`,
  `chickenTurn`, `chickenFeed`, `chickenGraze`; the keeper's job
  (`tryAssignTend`, `jobTend` and its three stages, `troughWants`,
  `feedScumFor`, `releaseTend`).
- [`internal/sim/crashpod.go`](../internal/sim/crashpod.go) — `podRareItem`,
  `podGun`, and the trough and pet in `arrive` (`podTrough`, `podPet`).
- [`internal/sim/systems.go`](../internal/sim/systems.go) — where tending sits
  in `assignWorkJob`; `chickenTurn` in `step`.
- [`internal/sim/world.go`](../internal/sim/world.go) (the `Trough` terrain),
  [`property.go`](../internal/sim/property.go) (a fixture with a depot),
  [`inventory.go`](../internal/sim/inventory.go) (the `Feed` item),
  [`entity.go`](../internal/sim/entity.go) (`Chicken`, `JobTend`, and the
  `keeper`/`trough` fields).
- [`internal/sim/config.go`](../internal/sim/config.go) — the `Chickens`
  section and the `crash-pod-*-weight` settings.
- [`internal/sim/chickens_test.go`](../internal/sim/chickens_test.go).
- [`internal/glyphs/glyphs.go`](../internal/glyphs/glyphs.go) — 🐔 and 🪣,
  appended to `All` so no existing glyph's wire index moved.

## How it works

### The chicken

`chickenTurn` runs once a tick, paced by `chicken-slowness`:

1. **Starve.** Like a rat, a chicken has only `NeedFood`
   (`chicken-hunger-rise`, a colonist's rate) and starves at the top of it,
   leaving an animal carcass.
2. **Eat**, once hungry (`SeekAt`): from its trough if it has feed and is
   reachable (`chickenFeed`). It eats one unit, on whoever's ledger line holds
   some (`takeFeed`), so a dead keeper's feed still feeds the hen. Otherwise
   it **grazes** exposed cave scum within `chicken-graze-radius`
   (`chickenGraze`), the way a peaceful alien does.
3. **Go home.** A chicken more than `chicken-roam` tiles from its trough walks
   back toward it; otherwise it wanders.

A chicken flees nothing and nothing hunts it. Cats hunt only rats (`catTurn`),
rats flee only cats, and a Hostile alien's prey is colonists, rats and other
aliens (`nearestReachablePrey`). Like a cat or a rat, a chicken on a tile a
colonist needs always gives way (`nudgeLoiterer`). A chicken added with the
spawn command is a stray: no keeper, no trough, so it lives on scum.

### The trough

`Trough` is a fixture with a depot, like a chest, holding only feed. It is
stamped in a keeper's pod in front of the bunk, private to the keeper, and
lands holding `trough-fill` feed. Nothing that hunts for a chest picks it:
every storage search filters on `Terrain == Storage`.

### Keeping chickens (`JobTend`)

A keeper tends once its trough holds fewer than `trough-low` units and some
chicken of its is alive (`troughWants`). It sets out for enough feed to bring
the trough back to `trough-fill`, in whole batches of `feedRecipe` (1 cave
scum to 4 feed) and never more than one scraped load (`feedScumFor`):

1. **Gather**: scrape scum into its own pockets from the nearest exposed
   patch, moving straight on to the next patch if one runs dry. If no scum is
   left, it mixes the whole batches it has.
2. **Mix**: at the nearest scumhouse, work `feedRecipe` on the scum in hand,
   all batches at once. Nothing goes through the depot.
3. **Fill**: carry the feed to the trough and credit it to the keeper.

Feed in a keeper's pockets goes to the trough whenever there is room for it,
so a keeper whose job was cut short doesn't carry feed around for good.

## Why it is this way

- **Feed is not in the recipe table.** A cook works the first recipe it has
  inputs for, so a feed recipe there would let the colony's cooks spend the
  scum it bought for meals on feed nobody ordered. Only keepers mix feed,
  from their own scum.
- **Keepers share the stove.** At first a keeper claimed the scumhouse like a
  cook (`workshopClaims`). But a colony cook keeps its claim for as long as
  there is scum to cook (`cooksOn`), so a keeper waiting for a free stove
  waited forever. On seed 3 (20 colonists), all four hens starved beside empty
  troughs while their keepers were alive. Mixing doesn't touch the depot, so
  it needs no claim.
- **Tending comes before construction.** A hen at an empty trough with no scum
  nearby starves within a few hundred ticks. With tending placed after the
  colony's cleaning, keepers were busy building or scraping for the colony
  when the trough ran low, and most hens died with their keepers alive. Tending
  is a few units of scum, and only once the trough is low, so it costs the
  colony little to put it first. A gather leg that ended when one patch ran
  dry had the same effect: the keeper went back to the queue for its next
  assignment while the hens went hungry.
- **The trough lands full.** It used to land empty, and two hens on seed 9
  starved in their first 1,500 ticks while their keepers settled in.
- **Before the scum incubator, chickens cost colonists.** Hens and keepers
  draw on the same wild scum the colony scraped. Over 20 colonists on a 200×200
  map, seeds 1–16, 30,000 ticks, 33 colonists starved with chickens against 10
  without. Turning grazing off barely helped (28), so it was not only the
  hens. With the incubator ([incubator.md](./incubator.md)) the colony stops
  scraping wild scum routinely, and the same sweep starved 1 with chickens
  against 2 without; 66 hens were alive at the end, against 30 with grazing
  off.

## Extending it

- **Eggs**: a chicken whose hunger is low could lay an egg item into its
  trough's depot, on its keeper's account, as a new food the keeper eats or
  sells. Keep any chance on `World.rng` (it changes the simulation) and name
  the item in `isBiomatter` only if a scumhouse should cook it.
- **Breeding**: `tryMate`/`giveBirth` are rat-only (`canBreed` checks the
  kind). Chicks would need a keeper and trough copied from the mother.
- **Feed for sale**: a keeper with surplus could ask a price for it. Feed is
  an ordinary item with a ledger, so the order book would take it. The
  missing part is a buyer.
- A cat's `keeper` is only displayed today. Pet bonds (mood, grief on death)
  belong in the cognition config as a reaction on the keeper.

## Related

- [crash-pods.md](./crash-pods.md) — the rare item and the pod layout.
- [entities-and-ai.md](./entities-and-ai.md) — the creature roster and turns.
- [scumhouse.md](./scumhouse.md) — scum, scraping, and the stove.
- [incubator.md](./incubator.md) — why wild scum stopped running out.
- [property.md](./property.md) — private fixtures and ledgers.
