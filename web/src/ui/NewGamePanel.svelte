<script lang="ts">
  // New game: the settings a game starts from, seeded from the URL
  // (?width=…&seed=…&fog-of-war=false). Keys are mars-sim.yaml's. A cold load
  // opens this tab rather than starting a game, so the player sees the
  // settings first and the defaults are one click from landing the ships.
  // Below it, saving the running game to a file and loading one back
  // (docs/save-load.md).
  import { loadGame, newGame, saveGame, ui } from '../game.svelte';
  import { describeShips, initialSettings, shipLoads } from '../settings';
  import type { Settings } from '../sim/client';

  const init = initialSettings();
  let width = $state(Number(init.width));
  let height = $state(Number(init.height));
  let colonists = $state(Number(init.colonists));
  // A number input binds a number, or null when empty: empty lets the engine pick.
  let seed: number | null = $state(init.seed === undefined ? null : Number(init.seed));
  let fog = $state(init['fog-of-war'] !== false);
  // Off (the default): the player draws zones and colonists build only in
  // them. On: the colony zones and builds by itself (docs/zoning.md).
  let autoZoning = $state(init['zoning-auto'] === true);
  // The founders' ships, which the player lands one by one after Start
  // (docs/ships.md).
  const ships = $derived(describeShips(shipLoads(colonists)));

  function start(e: SubmitEvent) {
    e.preventDefault();
    const s: Settings = { width, height, colonists, 'fog-of-war': fog, 'zoning-auto': autoZoning };
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

<form onsubmit={start}>
  <p class="muted">Choose a world and its founders, then land their ships.</p>
  <label>Width <input type="number" min="64" bind:value={width} /></label>
  <label>Height <input type="number" min="64" bind:value={height} /></label>
  <label>Colonists <input type="number" min="1" bind:value={colonists} aria-describedby="ship-count" /></label>
  <p id="ship-count" class="muted ships" aria-live="polite">{ships} to land</p>
  <label>Seed <input type="number" placeholder="random" bind:value={seed} /></label>
  <label class="check"><input type="checkbox" bind:checked={fog} /> Fog of war</label>
  <label class="check"><input type="checkbox" bind:checked={autoZoning} /> Colonists zone for themselves</label>
  <button type="submit">Start</button>
</form>

<section aria-label="Saved games">
  <h3>Saved games</h3>
  <p class="muted">A save is the whole game: it plays on exactly as it would have. Saves only load into the build that made them, or one whose world has not changed shape since.</p>
  <div class="row">
    <button type="button" onclick={saveGame} disabled={!ui.hello} title="Save game (Ctrl+S / ⌘S)" aria-keyshortcuts="Control+S Meta+S">Save game</button>
    <button type="button" onclick={() => picker.click()}>Load game…</button>
  </div>
  <input type="file" accept=".marssave" bind:this={picker} onchange={picked} hidden />
</section>

<style>
  form { display: grid; gap: 10px; }
  section { display: grid; gap: 8px; margin-top: 18px; padding-top: 12px; border-top: 1px solid var(--line); }
  h3 { margin: 0; font-size: 1em; }
  .row { display: flex; gap: 8px; flex-wrap: wrap; }
  p { margin: 0; }
  .muted { color: var(--muted); }
  .ships { margin-top: -6px; font-size: 0.9em; }
  label { display: grid; gap: 3px; color: var(--muted); }
  label.check { display: flex; gap: 8px; align-items: center; color: var(--fg); }
  input[type='number'] { width: 100%; }
  button[type='submit'] { background: var(--accent); border-color: var(--accent); color: #fff; justify-self: start; }
</style>
