<script lang="ts">
  // Picks a series to add to a chart: a metric, then (for a metric read per
  // good or per account) its subject. A colony of hundreds has hundreds of
  // wallets, so subjects can be filtered by name.
  import { pickerOrder, SUBJECT_NOUN, type Catalog } from './builder';

  let { cat, taken, onpick, oncancel }: {
    cat: Catalog;
    /** Series already on the chart, not offered again. */
    taken: string[];
    onpick: (key: string) => void;
    oncancel: () => void;
  } = $props();

  let metric = $state(-1);
  let filter = $state('');
  let subject = $state('');

  const groups = $derived([...new Set(cat.metrics.map((m) => m.group))]);
  const m = $derived(metric >= 0 ? cat.metrics[metric] : undefined);
  const subjects = $derived.by(() => {
    if (metric < 0) return [];
    const f = filter.trim().toLowerCase();
    return pickerOrder(cat.metrics[metric].per, cat.series
      .filter((s) => s.metric === metric && !taken.includes(s.key))
      .filter((s) => !f || (s.subject ?? '').toLowerCase().includes(f)));
  });
  // A colony metric has one series: pick it as soon as the metric is chosen.
  $effect(() => {
    if (m?.per === 'colony') subject = subjects[0]?.key ?? '';
    else if (!subjects.some((s) => s.key === subject)) subject = subjects[0]?.key ?? '';
  });
</script>

<div class="picker">
  <select aria-label="Metric" bind:value={metric}>
    <option value={-1} disabled>Choose a measure…</option>
    {#each groups as g (g)}
      <optgroup label={g}>
        {#each cat.metrics as x, i (x.key)}
          {#if x.group === g}<option value={i}>{x.label}{x.per === 'colony' ? '' : ` (by ${SUBJECT_NOUN[x.per]})`}</option>{/if}
        {/each}
      </optgroup>
    {/each}
  </select>
  {#if m}
    <p class="doc">{m.doc}{m.kind === 'total' ? ' · a running total' : ''}</p>
    {#if m.per !== 'colony'}
      {#if m.per === 'account'}<input type="search" placeholder="Filter by name" aria-label="Filter accounts" bind:value={filter} />{/if}
      {#if subjects.length}
        <select aria-label={SUBJECT_NOUN[m.per]} bind:value={subject} size={Math.min(6, Math.max(2, subjects.length))}>
          {#each subjects as s (s.key)}<option value={s.key}>{s.subject}{s.ended ? ' (gone)' : ''}</option>{/each}
        </select>
      {:else}
        <p class="doc">{filter ? 'No match.' : m.per === 'fixture' ? 'Nothing measured yet: a kind appears once one is built.' : m.per === 'skill-rank' ? 'Nothing measured yet: a rank appears once a colonist holds it.' : 'Nothing measured yet: a good appears once it has been traded or stored.'}</p>
      {/if}
    {:else if !subjects.length}
      <p class="doc">Already on this chart, or no reading yet: the first sample comes at the top of the hour.</p>
    {/if}
  {/if}
  <div class="actions">
    <button type="button" disabled={!subject} onclick={() => onpick(subject)}>Add</button>
    <button type="button" class="quiet" onclick={oncancel}>Cancel</button>
  </div>
</div>

<style>
  .picker { display: flex; flex-direction: column; gap: 6px; padding: 8px; margin-top: 6px; border: 1px solid rgba(255, 255, 255, 0.1); border-radius: 6px; }
  select, input { width: 100%; box-sizing: border-box; }
  .doc { margin: 0; font-size: 12px; color: #c3c2b7; }
  .actions { display: flex; gap: 6px; }
  .quiet { background: transparent; }
</style>
