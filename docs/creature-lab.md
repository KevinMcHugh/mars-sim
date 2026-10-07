# Creature Lab

> Part of the [mars-sim documentation](./README.md).

## What it is

Creature Lab is a separate server, `creature-lab`, that keeps a catalog of alien species in Postgres. The game's own roster code rolls each species, and each one carries an SVG map sprite for every form of its life (egg, grub, adult, queen…; most species have a single form). Species are created one at a time, whenever someone wants one. Sprites are drawn whenever someone gets to it, usually by Claude over MCP. Art is paid for once per species in the catalog, never per world generated. It is modelled on the [inventory](https://github.com/KevinMcHugh/inventory) app's infrastructure and deploys to a sprites.dev sprite named `creature-lab`.

The game does not read the catalog yet. See "Extending it" for the plan.

## Source

- [`cmd/creature-lab/main.go`](../cmd/creature-lab/main.go): the binary. `serve` runs the server, and `keys create|list|rotate|delete` manages api keys.
- [`internal/sim/species_lab.go`](../internal/sim/species_lab.go): `RollLabSpecies(seed)`, plus exported `FeaturePhrases` and `SizeWords`. This file is the lab's only door into the simulation. [`species_lab_test.go`](../internal/sim/species_lab_test.go) pins that a lab species equals the species a real world rolls.
- [`internal/creaturelab/species.go`](../internal/creaturelab/species.go): rolling, `Traits` (a species in words), `Forms` (the sprite slots a lifecycle implies), `HouseStyle`, and `Brief` (what an artist draws a slot from).
- [`internal/creaturelab/svg.go`](../internal/creaturelab/svg.go): `CheckSVG` refuses active content. `Lint` gives house-style advice.
- [`internal/creaturelab/service.go`](../internal/creaturelab/service.go): `Service` holds every operation and its view models. REST, MCP and the pages all call it.
- [`internal/creaturelab/labmcp/`](../internal/creaturelab/labmcp/labmcp.go): the MCP tools and the server's `Instructions`.
- [`internal/creaturelab/web/`](../internal/creaturelab/web/): the router (`server.go`), the JSON API (`api.go`), the pages (`pages.go`, `templates/`), and [`e2e_test.go`](../internal/creaturelab/web/e2e_test.go), which covers the API, OAuth, the pages and a real MCP client session against Postgres.
- [`internal/creaturelab/auth/`](../internal/creaturelab/auth/), [`oauth/`](../internal/creaturelab/oauth/oauth.go): keys, tokens, middleware, and the OAuth 2.1 server claude.ai needs.
- [`internal/creaturelab/db/`](../internal/creaturelab/db/): sqlc output. **Do not edit.** Run `make generate` in `creature-lab/` instead.
- [`creature-lab/`](../creature-lab/): the non-Go half: `db/migrations` (dbmate), `db/queries` (sqlc), `sqlc.yaml`, the `Makefile`, and `deploy/run-postgres.sh`.

## How it works

### The loop

1. **Preview** (`preview_species`, or the Roll species page). This rolls species from seeds without saving anything. It is free, so browse a dozen.
2. **Keep** (`create_species`). The lab rolls the seed again and stores the result.
3. **Draw each form.** `get_sprite_brief` returns the house style, the species' field notes, the form's exact body (eyes, arms, legs, tail, wings, graded features, size, inert or not), and the species' already-accepted sprites, so a life reads as one creature. The artist draws, then calls `submit_sprite_candidate`. Each submission is a new candidate, so iterating never loses an earlier version.
4. **Accept** (`accept_sprite_candidate`, or the Accept button). At most one candidate per slot is accepted. A species is *complete* when every slot has one.
5. **Export** (`GET /api/export`). Each complete species comes out as rolled, with its accepted SVGs.

### Who pays for drawing

The server never calls a model. Over MCP, the person's own Claude draws, so a sprite costs whatever their Claude plan costs, when they choose to spend it. Without MCP, the species page has **Brief as text** (`/brief/{id}/{form}`) to paste into any chat, and a **Paste an SVG** box for the answer. Contrast the Scum Lab [Sprite Designer](./sprite-designer.md), which bills a Console API key per token from the browser.

### Schema

The schema is dedicated to this job: purpose-built tables, not inventory's generic kinds and models. It follows inventory's conventions: xid ids in `CHAR(20)`, soft delete through `deleted_at` on every live table, and queries that filter it.

| table | holds |
| --- | --- |
| `species` | `seed`, `generator_rev` (the commit that rolled it), `data` (the whole `sim.AlienSpecies` as JSONB), copies of the name, temperament and description for listing, `form_count` (its sprite slots), `notes` |
| `sprite_candidates` | `(species_id, form)` slot, `svg` in a `text` column, `note`, `author`, `accepted_at`. A partial unique index allows one accepted per live slot. |
| `api_keys`, `oauth_*` | auth (below) |

A **slot** is a form index, the same as the form's index in `AlienSpecies.LifeForms()`. A single-form species has one slot, 0, named "adult". Castes are separate slots, because each one draws on the map as its own creature.

### Auth

There are no tenants. The catalog is one shared set of species, and the api key that made something is recorded on it (`created_by`). Credentials:

- **`cl_` api keys**, minted with `creature-lab keys create --name you`. Send one as `Authorization: Bearer cl_…` from scripts or Claude Code.
- **OAuth** for claude.ai custom connectors: dynamic client registration, PKCE, and a page where the user pastes a `cl_` key. That issues a `cl_at_` access token (24 h) and a single-use `cl_rt_` refresh token (30 d). This is inventory's flow without tenants or Google sign-in.
- **Web sessions**: `/login` takes a key and sets an HttpOnly, SameSite=Lax cookie holding a 7-day `cl_at_` token with no client.

Deleting a key cuts off every token and session it approved, because their lookups join on a live key.

### Surfaces

| | path | auth |
| --- | --- | --- |
| MCP (streamable HTTP) | `/mcp/rpc` | bearer |
| JSON API | `/api/...`, one route per MCP tool, plus `/api/export` | bearer or cookie |
| Pages | `/`, `/preview`, `/species/{id}` | cookie |
| Sprite images | `/sprites/{candidateId}.svg` | either |
| OAuth and health | `/.well-known/*`, `/oauth/*`, `/health` | public |

## Why it is this way

- **It lives in mars-sim, not its own repo.** It has to call the game's species roller, and Go's `internal/` rule means only code inside this module can import `internal/sim`. A separate repo would mean exporting the roster code as a public package, or a copy that drifts. Its deps (chi, pgx, the MCP SDK) are in `go.mod` now, but the game binaries and the WASM build do not import them.
- **The species is stored whole, not re-derived from its seed.** Features keep landing in the roster code, and each one can change what a seed rolls. In this very change, main gained feature-based names and apex species. A sprite drawn for "seed 5" must stay attached to the creature it was drawn from, so `data` keeps the species as rolled and `generator_rev` records which code rolled it. That is also why species are created on demand rather than in bulk: a pile rolled today would be missing next month's features. `data` JSON round-trips the struct exactly (`TestSpeciesJSONRoundTrip`). A field added later decodes as its zero value on old rows.
- **Lab species are world species.** `RollLabSpecies` is `rollAlienSpeciesRoster` with one species and default config, so a catalog species is exactly what a world with that seed and `alien-species-count: 1` rolls. `TestRollLabSpeciesIsTheWorldsSpecies` checks this against a real world.
- **Seeds stay below 2^31.** They travel as JSON numbers to MCP clients and browsers, where integers past 2^53 lose precision silently. Short seeds are also readable aloud. An explicit seed may be any int64.
- **Unsafe SVG is refused, not just linted.** The Sprite Designer only warns, because it renders sprites through `<img>`. The lab also *serves* sprites from the same origin as its session cookie, and a browser that navigates straight to an SVG document runs its scripts. So `CheckSVG` rejects scripts, `on*` handlers, `<foreignObject>`, `<image>`, non-`#` hrefs, external `url()`/`@import` and DTDs. On top of that, sprite responses carry a `sandbox` CSP, and the pages run no script at all (`default-src 'none'`). House style (viewBox, `<text>`, size) stays advice.
- **No codegen for the API, no React app.** Inventory generates its REST layer from OpenAPI and ships a Vite/React UI. Here the surfaces are small, so one `Service` with view-model returns keeps REST, MCP and pages in step: the same "one truth" idea without the generator. Server-rendered pages mean the binary is the whole deploy, with no `npm` step on the sprite. sqlc and dbmate stay, run with `go run …@version` from the Makefile so their dependency trees stay out of mars-sim's `go.mod`.
- **The MCP endpoint is `/mcp/rpc`, not `/mcp`.** As inventory found, the sprites.dev proxy intercepts the bare `/mcp` path and POSTs hang.
- **An empty `{{define}}` does not override a `{{block}}`.** Go templates treat a whitespace-only definition as empty and keep the original. The login page first did this, so it still had the layout's Sign out button, and clicking "Sign in" signed out. Give an override some content.

## Deploy (sprites.dev)

The recipe is inventory's [deploy.md](https://github.com/KevinMcHugh/inventory/blob/main/docs/deploy.md), adapted. The sprite needs Go 1.27.1 or newer (the module's `go` line); `go build` fetches that toolchain itself if the installed Go is older.

```sh
sprite create --skip-console creature-lab
sprite use creature-lab
sprite exec -- bash -lc 'sudo apt-get update -qq && sudo apt-get install -y -qq postgresql postgresql-contrib git'
sprite exec -- bash -lc 'sudo curl -fsSL -o /usr/local/bin/dbmate \
  https://github.com/amacneil/dbmate/releases/latest/download/dbmate-linux-amd64 && sudo chmod +x /usr/local/bin/dbmate'
sprite exec -- bash -lc 'cd ~ && git clone https://github.com/KevinMcHugh/mars-sim.git'

# Postgres as a sprite service. The launcher recreates /var/run/postgresql,
# which is tmpfs on the sprite image and is wiped on every reboot.
sprite exec -- bash -lc 'sudo install -m 0755 ~/mars-sim/creature-lab/deploy/run-postgres.sh /usr/local/bin/run-postgres.sh'
sprite exec -- bash -lc '/.sprite/bin/sprite-env services create postgres --cmd /usr/local/bin/run-postgres.sh --no-stream'
sprite exec -- bash -lc 'sudo -u postgres createuser -s sprite; sudo -u postgres createdb -O sprite creaturelab'

sprite exec -- bash -lc "echo 'export DATABASE_URL=\"postgres:///creaturelab?host=/var/run/postgresql&sslmode=disable\"' >> ~/.profile"
sprite exec -- bash -lc 'cd ~/mars-sim/creature-lab && dbmate --migrations-dir db/migrations --no-dump-schema up && \
  cd .. && go build -o creature-lab/bin/creature-lab ./cmd/creature-lab'
sprite exec -- bash -lc '~/mars-sim/creature-lab/bin/creature-lab keys create --name kev'   # SAVE THE KEY

# PUBLIC_URL must be the https URL, because OAuth discovery advertises it.
sprite exec -- bash -lc '/.sprite/bin/sprite-env services create creature-lab \
  --cmd /home/sprite/mars-sim/creature-lab/bin/creature-lab --dir /home/sprite/mars-sim \
  --needs postgres --http-port 8080 \
  --env "DATABASE_URL=postgres:///creaturelab?host=/var/run/postgresql&sslmode=disable,PORT=8080,PUBLIC_URL=https://creature-lab-<org>.sprites.app" \
  --no-stream'
sprite config update -s creature-lab --url-auth public
```

**Redeploy:** `git pull`, then `dbmate … up`, `go build …`, and `sprite-env services restart creature-lab`. A redeploy changes `generator_rev` for new species only. Existing rows keep the species they were rolled as.

**Connect Claude:** in claude.ai, add a custom connector at `https://creature-lab-<org>.sprites.app/mcp/rpc` with automatic client registration, then paste a `cl_` key when asked. For Claude Code: `claude mcp add --transport http creature-lab https://…/mcp/rpc --header "Authorization: Bearer cl_…"`.

**Local:** `cd creature-lab && DATABASE_URL=postgres://localhost/creaturelab?sslmode=disable make dev`. `make key NAME=you` mints a key. To include the Postgres tests, set `CREATURE_LAB_TEST_DATABASE_URL` to a scratch database **the tests may wipe**; without it, `go test ./...` skips them.

## Extending it

- **Using the catalog in the game** is the point, and it is not built yet. The likely shape:
  1. A `-species-pack` file: an export (`/api/export`) the game loads in place of rolling, with `AlienSpeciesCount` picking from it on the world's RNG. Only species with a sprite for every form are picked.
  2. The wire saying which species and form an alien is.
  3. `buildAtlas` drawing the pack's SVGs into atlas cells, with the emoji as fallback (see [sprite-designer.md](./sprite-designer.md#using-a-sprite-in-the-game)).

  Because stored species are frozen, a pack is stable across game commits. The roster code's later features will not reach old species unless they are re-rolled.
- **A new field on the stored species** needs no migration: it lives in `data`. Add it to `Traits` if a person or an artist should see it, and to `Brief` if it changes the drawing, as `Apex` does.
- **A new MCP tool:** add a `Service` method, then a tool in `labmcp`, a route in `web/api.go`, and a case in `e2e_test.go`.
- **Schema change:** add a dbmate migration (never edit an applied one), then run `make generate`.
- **Server-side generation**, if wanted later, belongs behind its own budget: an `ANTHROPIC_API_KEY`, a per-month cap stored in the database, and the cost recorded on each candidate. The lab deliberately has none today.
- Keep `HouseStyle` in step with the Sprite Designer's `SYSTEM_PROMPT`.

## Related

- [lore.md](./lore.md), [alien-lifecycles.md](./alien-lifecycles.md), [alien-taxonomy.md](./alien-taxonomy.md): what a species is, and the forms that become slots.
- [sprite-designer.md](./sprite-designer.md): the browser tool for drawing one sprite, and what the map needs before it can use one.
- [determinism.md](./determinism.md): the lab rolls on the game's lore streams, and picks its own random seeds off them.
