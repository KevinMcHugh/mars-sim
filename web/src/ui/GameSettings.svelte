<script lang="ts">
  // Every setting the running game was started with, from the "config" topic
  // (internal/wire/config.go), grouped as mars-sim.yaml groups them, each
  // beside its default. Read-only: the new-game form above sets only a few;
  // the browser reads no mars-sim.yaml, so the rest are DefaultConfig's (or
  // whatever a loaded save was started with). See
  // docs/game-settings.md.
  import { subscribe, topics } from '../game.svelte';
  import Section from './Section.svelte';

  type Value = number | boolean | null;
  interface Setting { key: string; doc: string; value: Value; default: Value }
  interface ConfigTopic { seed: number; sections: { title: string; settings: Setting[] }[] }

  $effect(() => subscribe('config'));
  const cfg = $derived(topics.data.config as ConfigTopic | undefined);

  let filter = $state('');
  let changedOnly = $state(false);

  const changed = (s: Setting) => s.value !== s.default;
  const total = $derived(cfg?.sections.reduce((n, sec) => n + sec.settings.length, 0) ?? 0);
  const nChanged = $derived(cfg?.sections.reduce((n, sec) => n + sec.settings.filter(changed).length, 0) ?? 0);

  // A filter matches a key, a description or a section title.
  const shown = $derived.by(() => {
    if (!cfg) return [];
    const q = filter.trim().toLowerCase();
    return cfg.sections
      .map((sec) => ({
        title: sec.title,
        settings: sec.settings.filter((s) =>
          (!changedOnly || changed(s)) &&
          (!q || s.key.includes(q) || s.doc.toLowerCase().includes(q) || sec.title.toLowerCase().includes(q))),
      }))
      .filter((sec) => sec.settings.length > 0);
  });
  // Groups start shut (there are a couple of hundred settings), and open
  // themselves while a search or the changed-only box narrows the list.
  const narrowed = $derived(filter.trim() !== '' || changedOnly);

  function fmt(v: Value): string {
    if (typeof v === 'boolean') return v ? 'on' : 'off';
    if (typeof v === 'number') return v.toLocaleString();
    return '—';
  }
</script>

<Section id="game.settings" tag="h2" title="Settings" note={cfg ? `(${total}, ${nChanged} changed)` : undefined}>
  {#if !cfg}
    <p class="muted">Start or load a game to see the settings it runs with.</p>
  {:else}
    <p class="muted">What this game runs with. The form above sets a few; the rest are the built-in defaults, or whatever a loaded save was started with. A highlighted setting differs from its default.</p>
    <dl class="seed"><dt>seed</dt><dd><code>{cfg.seed}</code></dd></dl>
    <div class="controls">
      <input type="search" placeholder="Filter settings" aria-label="Filter settings" bind:value={filter} />
      <label class="check"><input type="checkbox" bind:checked={changedOnly} /> Changed only</label>
    </div>
    {#if shown.length === 0}
      <p class="muted">No settings match.</p>
    {/if}
    {#each shown as sec (sec.title)}
      <details open={narrowed}>
        <summary>{sec.title} <span class="muted">({sec.settings.length})</span></summary>
        <ul>
          {#each sec.settings as s (s.key)}
            <li class:changed={changed(s)}>
              <div class="row">
                <code class="key">{s.key}</code>
                <span class="value">{fmt(s.value)}</span>
              </div>
              {#if s.doc}<div class="doc">{s.doc}</div>{/if}
              {#if changed(s)}<div class="doc">default {fmt(s.default)}</div>{/if}
            </li>
          {/each}
        </ul>
      </details>
    {/each}
  {/if}
</Section>

<style>
  p { margin: 0 0 8px; line-height: 1.5; }
  .muted { color: var(--muted); }
  .seed { display: grid; grid-template-columns: max-content 1fr; gap: 3px 12px; margin: 0 0 8px; }
  .seed dt { color: var(--muted); }
  .seed dd { margin: 0; }
  .controls { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; margin-bottom: 8px; }
  .controls input[type='search'] { flex: 1; min-width: 10em; }
  label.check { display: flex; gap: 6px; align-items: center; }
  details { margin: 2px 0; }
  summary { cursor: pointer; padding: 3px 0; }
  ul { list-style: none; padding: 0 0 0 1.1em; margin: 2px 0 8px; display: grid; gap: 6px; }
  li { padding-left: 6px; border-left: 2px solid transparent; }
  li.changed { border-left-color: var(--accent); }
  .row { display: flex; justify-content: space-between; gap: 12px; }
  .key { overflow-wrap: anywhere; }
  .value { font-variant-numeric: tabular-nums; white-space: nowrap; }
  li.changed .value { color: var(--accent); font-weight: 600; }
  .doc { color: var(--muted); font-size: 0.85em; line-height: 1.35; }
</style>
