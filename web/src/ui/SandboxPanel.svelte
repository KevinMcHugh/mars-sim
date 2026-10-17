<script lang="ts">
  // The Sandbox tab: a debugging and tuning tool for a game started with
  // Sandbox ticked (docs/sandbox.md). Pick up a colonist or an alien of a
  // chosen species, then click open floor on the map to place one there; the
  // tool stays up for the next click. The species come from the lore topic,
  // in roster order, which is the index the place command takes.
  import { armPlace, subscribe, topics, ui } from '../game.svelte';
  import Help from './Help.svelte';

  let help = $state(false);

  $effect(() => subscribe('lore'));
  // Leaving the tab puts the tool down and clears its tint from the map.
  $effect(() => () => armPlace(null));

  const species = $derived((topics.data.lore as { species: { label: string }[] } | undefined)?.species ?? []);
  // The species an alien is placed as: an index into species, or null for
  // one at random. Kept when the tool is put down, for the next pick-up.
  let pick: number | null = $state(null);
  // A held alien tool follows the species picker.
  $effect(() => {
    const t = ui.placeTool;
    if (t?.kind === 'alien' && t.species !== pick) armPlace({ kind: 'alien', species: pick });
  });

  const held = $derived(ui.placeTool?.kind);
  function press(kind: 'colonist' | 'alien') {
    if (held === kind) armPlace(null);
    else armPlace(kind === 'alien' ? { kind, species: pick } : { kind });
  }
  const what = $derived(
    held === 'colonist' ? 'a new colonist'
      : held === 'alien' ? (pick === null ? 'an alien of a random species' : `a ${species[pick]?.label ?? 'alien'}`)
      : '',
  );
</script>

{#if !ui.hello?.sandbox}
  <p class="muted">This game is not a sandbox. Start one from the Game tab with <em>Sandbox</em> ticked to place colonists and aliens by hand.</p>
{:else}
  <Help bind:open={help} float label="What is the sandbox?" />
  {#if help}<p class="help-text">A sandbox starts empty: nobody lands, no aliens wait in the caverns, and nothing arrives on its own. Place colonists and aliens on open floor the colony has found, then let the game run to see what they do. A placed colonist has no ship, so no bunk or locker of its own.</p>{/if}

  <div class="row">
    <button type="button" class:on={held === 'colonist'} aria-pressed={held === 'colonist'} onclick={() => press('colonist')}
      title="A new colonist with a random name, looks and traits">Colonist</button>
    <button type="button" class:on={held === 'alien'} aria-pressed={held === 'alien'} onclick={() => press('alien')}
      title="An alien of the species picked below">Alien</button>
  </div>
  <label class="field">
    Species
    <select bind:value={pick} disabled={species.length === 0}>
      <option value={null}>Random</option>
      {#each species as s, i (i)}
        <option value={i}>{s.label}</option>
      {/each}
    </select>
  </label>
  {#if held}
    <p class="prompt">Click open floor to place {what}. Esc puts the tool down.</p>
  {:else}
    <p class="muted">Pick up a colonist or an alien, then click the map. It goes on the level the map shows; a refusal says why in the log.</p>
  {/if}
{/if}

<style>
  p { margin: 0 0 8px; }
  .muted { color: var(--muted); }
  .row { display: flex; gap: 6px; margin: 8px 0; }
  .row button.on { background: var(--accent); color: #fff; }
  .field { display: flex; align-items: center; gap: 6px; margin: 6px 0; }
  .field select { flex: 1; min-width: 0; }
  .prompt { color: var(--accent); }
</style>
