<script lang="ts">
  // The Sandbox tab: a debugging and tuning tool for a game started with
  // Sandbox ticked (docs/sandbox.md). Pick up a colonist or an alien of a
  // chosen species, then click open floor on the map to place one there; the
  // tool stays up for the next click. The species come from the lore topic,
  // in roster order, which is the index the place command takes. They are
  // searched rather than listed in a dropdown: a sandbox's roster is the
  // whole species pack, which only grows (matchSpecies, sandbox.ts).
  import { armPlace, subscribe, topics, ui } from '../game.svelte';
  import Help from './Help.svelte';
  import { matchSpecies, TEMPERAMENTS, type SpeciesLine } from './sandbox';

  let help = $state(false);

  $effect(() => subscribe('lore'));
  // Leaving the tab puts the tool down and clears its tint from the map.
  $effect(() => () => armPlace(null));

  const species = $derived((topics.data.lore as { species: SpeciesLine[] } | undefined)?.species ?? []);
  let query = $state('');
  // The temperament chips switched on; none on shows every temperament.
  let moods: string[] = $state([]);
  const toggleMood = (t: string) => { moods = moods.includes(t) ? moods.filter((m) => m !== t) : [...moods, t]; };
  const filtering = $derived(query.trim() !== '' || moods.length > 0);
  const matches = $derived(matchSpecies(species, query, moods));
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
  // Choosing a species picks up the alien tool with it: choosing is what
  // the player does just before placing one.
  function choose(i: number | null) {
    pick = i;
    armPlace({ kind: 'alien', species: i });
  }
  // Enter in the search box chooses the first match.
  function searchKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && matches.length > 0) {
      e.preventDefault();
      choose(matches[0]);
    }
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
    <input type="search" placeholder="Search {species.length} species" bind:value={query} onkeydown={searchKey}
      disabled={species.length === 0} aria-controls="sandbox-species" />
  </label>
  <div class="chips" role="group" aria-label="Temperament">
    {#each TEMPERAMENTS as t (t)}
      <button type="button" class:on={moods.includes(t)} aria-pressed={moods.includes(t)} onclick={() => toggleMood(t)}>{t}</button>
    {/each}
  </div>
  <div class="species" id="sandbox-species" role="listbox" aria-label="Alien species">
    {#if !filtering}
      <button type="button" role="option" aria-selected={pick === null} class:on={pick === null} onclick={() => choose(null)}>Random</button>
    {/if}
    {#each matches as i (i)}
      <button type="button" role="option" aria-selected={pick === i} class:on={pick === i} onclick={() => choose(i)}
        title={species[i].scientificName}>{species[i].label}</button>
    {:else}
      <p class="muted none">No species matches.</p>
    {/each}
  </div>
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
  .field input { flex: 1; min-width: 0; }
  .species { display: flex; flex-direction: column; max-height: 220px; overflow-y: auto; margin: 0 0 8px; border: 1px solid var(--line); border-radius: 4px; }
  .species button { text-align: left; border: none; border-radius: 0; background: transparent; padding: 4px 8px; }
  .species button:hover { background: rgba(255, 255, 255, 0.08); }
  .species button.on { background: var(--accent); color: #fff; }
  .none { margin: 0; padding: 4px 8px; }
  .chips { display: flex; gap: 4px; margin: 0 0 6px; }
  .chips button { padding: 1px 8px; font-size: 12px; text-transform: capitalize; }
  .chips button.on { background: var(--accent); color: #fff; }
  .prompt { color: var(--accent); }
</style>
