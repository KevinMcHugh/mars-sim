<script lang="ts">
  // The Game tab. First, saving the running game to a file and loading one
  // back (docs/save-load.md): the everyday use of the tab once a game runs.
  // Then New game, a foldable section (Section): the settings a game starts
  // from, seeded from the URL (?width=…&seed=…&fog-of-war=false). Keys are
  // mars-sim.yaml's. A cold load opens this tab rather than starting a game,
  // so the player sees the settings first and the defaults are one click
  // from landing the ships; the settings themselves sit folded shut under
  // Options, with a line above Start saying what the defaults give. Then every setting the running game uses
  // (GameSettings, docs/game-settings.md). At the bottom, a link to Scum Lab,
  // which Pages publishes beside the game (docs/scum-lab.md); the relative URL
  // finds it there, not under `npm run dev`.
  import { loadGame, newGame, saveGame, ui } from '../game.svelte';
  import GameSettings from './GameSettings.svelte';
  import Help from './Help.svelte';
  import Section from './Section.svelte';
  import { describeShips, initialSettings, POWER_POLICIES, shipLoads } from '../settings';
  import type { Settings } from '../sim/client';

  const init = initialSettings();
  let width = $state(Number(init.width));
  let height = $state(Number(init.height));
  let colonists = $state(Number(init.colonists));
  // How many alien species the world rolls (docs/lore.md).
  let species = $state(Number(init['alien-species-count']));
  // How deep the colony may dig: 1 keeps it on the landing level
  // (docs/z-levels.md); deeper levels are richer and more dangerous.
  let deepest = $state(Number(init['deepest-level']));
  // A number input binds a number, or null when empty: empty lets the engine pick.
  let seed: number | null = $state(init.seed === undefined ? null : Number(init.seed));
  let fog = $state(init['fog-of-war'] !== false);
  // Off (the default): the player draws zones and colonists build only in
  // them. On: the colony zones and builds by itself (docs/zoning.md).
  let autoZoning = $state(init['zoning-auto'] === true);
  // Off (the default): the player picks the tile of every stair, shaft,
  // hole and ladder. On: the colony may pick, and digs stairs down on its
  // own (docs/siting.md).
  let autoSiting = $state(init['siting-auto'] === true);
  // Who runs the colony's reactors (docs/power.md); the Trade tab can change
  // it during the game.
  let policy = $state(String(init['power-policy']));
  const policyLabel: Record<string, string> = {
    monopoly: 'Monopoly: the colony',
    market: 'Market: colonists who can afford one',
    'public-option': 'Public option: a market, and a colony reactor of last resort',
    charter: 'Charter: one corporation from back home',
  };
  // On: an empty world for debugging, filled by hand from the Sandbox tab
  // (docs/sandbox.md). The colonist and species counts are hidden then: the
  // engine ignores the first (nobody lands, and the landing cavern is sized
  // for one ship) and takes every species in the pack instead of the second.
  let sandbox = $state(init.sandbox === true);
  // The founders' ships, which the player lands one by one after Start
  // (docs/ships.md).
  let saveHelp = $state(false);
  const ships = $derived(describeShips(shipLoads(colonists)));

  function start(e: SubmitEvent) {
    e.preventDefault();
    const s: Settings = { width, height, colonists, 'alien-species-count': Math.max(1, species || 1), 'fog-of-war': fog, 'zoning-auto': autoZoning, 'siting-auto': autoSiting, 'deepest-level': Math.max(1, deepest || 1), sandbox, 'power-policy': policy };
    if (seed !== null && Number.isFinite(seed)) s.seed = seed;
    newGame(s);
  }

  // The file picker is hidden behind a button; each pick loads at once.
  let picker: HTMLInputElement;
  function picked() {
    const f = picker.files?.[0];
    if (f) loadGame(f);
    picker.value = ''; // so picking the same file again loads it again
  }
</script>

<section aria-label="Saved games">
  <h3>Saved games<Help bind:open={saveHelp} label="What is in a save?" /></h3>
  {#if saveHelp}<p class="help-text">A save is the whole game: it plays on exactly as it would have. Saves only load into the build that made them, or one whose world has not changed shape since.</p>{/if}
  <div class="row">
    <button type="button" onclick={saveGame} disabled={!ui.hello} title="Save game (Ctrl+S / ⌘S)" aria-keyshortcuts="Control+S Meta+S">Save game</button>
    <button type="button" onclick={() => picker.click()}>Load game…</button>
  </div>
  <input type="file" accept=".marssave" bind:this={picker} onchange={picked} hidden />
</section>

<section aria-label="New game">
  {#snippet newHelp()}Choose a world and its founders, then land their ships.{/snippet}
  <Section id="game.new" tag="h2" title="New game" help={newHelp}>
    <form onsubmit={start}>
      <p class="muted" aria-live="polite">
        {#if sandbox}An empty sandbox: place colonists and aliens of any species by hand{:else}{colonists} colonists in {ships}, {species} alien species{/if}
      </p>
      <Section id="game.new.options" title="Options" shut>
        <div class="options">
          <label>Width <input type="number" min="64" bind:value={width} /></label>
          <label>Height <input type="number" min="64" bind:value={height} /></label>
          {#if !sandbox}
            <label>Colonists <input type="number" min="1" bind:value={colonists} aria-describedby="ship-count" /></label>
            <p id="ship-count" class="muted ships" aria-live="polite">{ships} to land</p>
            <label>Alien species <input type="number" min="1" bind:value={species} /></label>
          {/if}
          <label title="How many levels down the colony may dig: 1 keeps it on the level its ships land on">Deepest level <input type="number" min="1" max="16" bind:value={deepest} /></label>
          <label>Seed <input type="number" placeholder="random" bind:value={seed} /></label>
          <label class="check"><input type="checkbox" bind:checked={fog} /> Fog of war</label>
          <label class="check"><input type="checkbox" bind:checked={autoZoning} /> Colonists zone for themselves</label>
          <label class="check" title="Colonists may choose where stairs, shafts, holes and ladders go, and dig stairs down on their own when they run out of rock"><input type="checkbox" bind:checked={autoSiting} /> Colonists choose where to dig down</label>
          <label title="Who builds and runs the reactors that charge the colony's batteries; the Trade tab can change it during the game">Power <select bind:value={policy}>{#each POWER_POLICIES as p (p)}<option value={p}>{policyLabel[p]}</option>{/each}</select></label>
          <label class="check" title="A debugging tool: nobody lands, no aliens or nests, nothing arrives on its own; place colonists and aliens by hand from the Sandbox tab"><input type="checkbox" bind:checked={sandbox} /> Sandbox: start empty, place creatures by hand</label>
        </div>
      </Section>
      <button type="submit">Start</button>
    </form>
  </Section>
</section>

<section aria-label="Settings">
  <GameSettings />
</section>

<section aria-label="Tools">
  <h3>Tools</h3>
  <p class="muted"><a href="scum-lab/" target="_blank" rel="noopener">Scum Lab</a>: tune cognition.yaml and see why a colonist picks their focus.</p>
</section>

<style>
  form, .options { display: grid; gap: 10px; }
  section { display: grid; gap: 8px; margin-top: 18px; padding-top: 12px; border-top: 1px solid var(--line); }
  section:first-child { margin-top: 0; padding-top: 0; border-top: none; }
  h3 { margin: 0; font-size: 1em; }
  .row { display: flex; gap: 8px; flex-wrap: wrap; }
  p { margin: 0; }
  .muted { color: var(--muted); }
  a { color: var(--accent); }
  .ships { margin-top: -6px; font-size: 0.9em; }
  label { display: grid; gap: 3px; color: var(--muted); }
  label.check { display: flex; gap: 8px; align-items: center; color: var(--fg); }
  input[type='number'] { width: 100%; }
  button[type='submit'] { background: var(--accent); border-color: var(--accent); color: #fff; justify-self: start; }
</style>
