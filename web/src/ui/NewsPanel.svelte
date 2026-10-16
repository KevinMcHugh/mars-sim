<script lang="ts">
  // The News tab: the colony's evening broadcasts, newest first, from the
  // "news" topic (internal/wire/news.go). Each evening at the news hour the
  // colony hears the day's major news (deaths, arrivals; docs/safety.md),
  // and every colonist awake hears it then, the rest when they wake. A story
  // about a place is a button that centers the map there, like a log line.
  import { goToLogLine, subscribe, topics } from '../game.svelte';
  import { logKindColor } from './logkinds';

  interface NewsItem { text: string; kind: string; place?: { x: number; y: number; level: number } }
  interface Broadcast { tick: number; day: number; when: string; items: NewsItem[] }
  interface News { hour: number; broadcasts: Broadcast[] }

  $effect(() => subscribe('news'));
  const news = $derived(topics.data.news as News | undefined);
  const hour = $derived(news && news.hour >= 0 ? `${String(news.hour).padStart(2, '0')}:00` : '');
</script>

{#if !news}
  <p class="muted">Loading…</p>
{:else if news.hour < 0}
  <p class="muted">The news is off (news-hour is −1).</p>
{:else}
  <p class="intro muted">The evening news airs at {hour} with the day's deaths and arrivals.</p>
  {#if news.broadcasts.length === 0}
    <p class="muted">Nothing has made the news yet.</p>
  {/if}
  {#each news.broadcasts as b (b.tick)}
    <section class="broadcast">
      <h3>Evening news, day {b.day} <span class="when">{b.when.split(" ").at(-1)}</span></h3>
      {#each b.items as it, i (i)}
        {#snippet body()}
          <span class="kind" style="color: {logKindColor(it.kind)}">{it.kind}</span>
          <span class="text">{it.text}</span>
        {/snippet}
        {#if it.place}
          <button type="button" class="item go" title="Show on the map ({it.place.x}, {it.place.y})"
            onclick={() => goToLogLine(it)}>{@render body()}</button>
        {:else}
          <div class="item">{@render body()}</div>
        {/if}
      {/each}
    </section>
  {/each}
{/if}

<style>
  .intro { margin: 0 0 8px; font-size: 12px; }
  .broadcast { margin-bottom: 12px; }
  h3 { font-size: 13px; margin: 0 0 4px; }
  .when { color: var(--muted); font-weight: normal; font-size: 11px; margin-left: 6px; font-variant-numeric: tabular-nums; }
  .item {
    display: grid; grid-template-columns: 5.5em 1fr; gap: 0 8px;
    padding: 3px 6px; border-radius: 4px; line-height: 1.4;
  }
  .item.go {
    width: 100%; text-align: left; font: inherit; color: inherit;
    background: none; border: none; cursor: pointer;
  }
  .item.go:hover { background: rgba(255, 255, 255, 0.09); }
  .item.go:hover .text { text-decoration: underline; text-decoration-color: var(--muted); }
  .kind { font-size: 12px; white-space: nowrap; padding-top: 1px; }
  .text { overflow-wrap: anywhere; }
  .muted { color: var(--muted); }
</style>
