<script lang="ts">
  // New game: the settings a game starts from, seeded from the URL
  // (?width=…&seed=…&fog-of-war=false). Keys are mars-sim.yaml's.
  import { newGame } from '../game.svelte';
  import { initialSettings } from '../settings';
  import type { Settings } from '../sim/client';

  const init = initialSettings();
  let width = $state(Number(init.width));
  let height = $state(Number(init.height));
  let colonists = $state(Number(init.colonists));
  // A number input binds a number, or null when empty: empty lets the engine pick.
  let seed: number | null = $state(init.seed === undefined ? null : Number(init.seed));
  let fog = $state(init['fog-of-war'] !== false);

  function start(e: SubmitEvent) {
    e.preventDefault();
    const s: Settings = { width, height, colonists, 'fog-of-war': fog };
    if (seed !== null && Number.isFinite(seed)) s.seed = seed;
    newGame(s);
  }
</script>

<form onsubmit={start}>
  <label>Width <input type="number" min="64" bind:value={width} /></label>
  <label>Height <input type="number" min="64" bind:value={height} /></label>
  <label>Colonists <input type="number" min="1" bind:value={colonists} /></label>
  <label>Seed <input type="number" placeholder="random" bind:value={seed} /></label>
  <label class="check"><input type="checkbox" bind:checked={fog} /> Fog of war</label>
  <button type="submit">Start</button>
</form>

<style>
  form { display: grid; gap: 10px; }
  label { display: grid; gap: 3px; color: var(--muted); }
  label.check { display: flex; gap: 8px; align-items: center; color: var(--fg); }
  input[type='number'] { width: 100%; }
  button[type='submit'] { background: var(--accent); border-color: var(--accent); color: #fff; justify-self: start; }
</style>
