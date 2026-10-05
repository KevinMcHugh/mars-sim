<script lang="ts">
  // The Recruit tab: grow the colony from off-world. Pay the recruiter from
  // the treasury for a set of candidates, shown side by side as cards, each
  // with the savings they would bring; tick any of them and hire them, turn
  // the set away, or pay again for a new set. Hires appear at once beside one
  // of the colony's ships. The fee and each hire's passage leave the colony's
  // money supply; a recruit's savings join it. See docs/recruiting.md.
  import { pickGlyph } from '../emoji';
  import { hireRecruits, rollRecruits, subscribe, topics } from '../game.svelte';
  import { money } from './format';

  interface Candidate {
    index: number; name: string; glyph: string; look?: string[];
    pronouns: string; orientation: string; age: number;
    height: string; heightCm: number; weightKg: number; skin: string; hair: string;
    traits: { name: string; desc: string }[];
    skills: { name: string; label: string; rank: number; maxRank: number }[];
    profession?: string; professionLabel?: string;
    savings: number;
  }
  interface Recruit {
    fee: number; cost: number; setSize: number; meals: number; treasury: number;
    offer: number; candidates: Candidate[]; ready: boolean; sets: number; hired: number;
  }

  $effect(() => subscribe('recruit'));

  const t = $derived(topics.data.recruit as Recruit | undefined);

  // The ticked cards, by index, for the set they were ticked in: a new set
  // starts with none ticked.
  let picked = $state<{ offer: number; picks: number[] }>({ offer: 0, picks: [] });
  const picks = $derived(t && picked.offer === t.offer ? picked.picks : []);

  function toggle(i: number): void {
    if (!t) return;
    const cur = picked.offer === t.offer ? picked.picks : [];
    picked = { offer: t.offer, picks: cur.includes(i) ? cur.filter((p) => p !== i) : [...cur, i].sort((a, b) => a - b) };
  }

  const cost = $derived(t ? picks.length * t.cost : 0);
  const brought = $derived(t ? t.candidates.filter((c) => picks.includes(c.index)).reduce((s, c) => s + c.savings, 0) : 0);

  // Why the hire cannot go, or null if it can.
  const blocked = $derived.by(() => {
    if (!t || picks.length === 0) return null;
    if (!t.ready) return 'Land the founders first: recruits arrive beside a ship.';
    if (cost > t.treasury) return `The treasury (${money(t.treasury)}) cannot pay ${money(cost)} for their passage.`;
    return null;
  });
  const canRoll = $derived(t !== undefined && t.setSize > 0 && t.fee <= t.treasury);

  function hire(): void {
    if (!t) return;
    hireRecruits(t.offer, picks);
    picked = { offer: 0, picks: [] };
  }
  function dismiss(): void {
    if (t) hireRecruits(t.offer, []);
  }
</script>

{#if !t}
  <p class="muted">Loading…</p>
{:else if t.setSize === 0}
  <p class="muted">Recruiting is off in this game (recruit-candidates is 0).</p>
{:else}
  <p class="muted">
    Hire colonists from off-world. A recruiter presents {t.setSize} candidates for {money(t.fee)}; each one
    you hire costs {money(t.cost)} for passage and arrives at once beside a ship with {t.meals}
    {t.meals === 1 ? 'meal' : 'meals'} and their own savings. The fee and passage leave the colony for good;
    the savings stay in the colonists' pockets.
  </p>
  <dl>
    <dt>Treasury</dt><dd>{money(t.treasury)}</dd>
    {#if t.hired > 0}<dt>Recruited</dt><dd>{t.hired} <span class="muted">from {t.sets} {t.sets === 1 ? 'set' : 'sets'}</span></dd>{/if}
  </dl>

  {#if t.offer === 0}
    <div class="row">
      <button type="button" class="go" disabled={!canRoll} onclick={rollRecruits}>Call the recruiter for {money(t.fee)}</button>
    </div>
    {#if !canRoll}<p class="warn">The treasury cannot pay the recruiter's {money(t.fee)}.</p>{/if}
  {:else}
    <p class="muted">Tick the candidates to hire. Pop this tab out and widen it to see them all side by side.</p>
    <ul class="cards">
      {#each t.candidates as c (c.index)}
        {@const on = picks.includes(c.index)}
        <li>
          <label class="card" class:on>
            <input type="checkbox" checked={on} onchange={() => toggle(c.index)} />
            <span class="head">
              <span class="glyph" aria-hidden="true">{pickGlyph(c.look, c.glyph)}</span>
              <span class="who">
                <strong>{c.name}</strong>
                <span class="muted">{c.pronouns} · age {c.age}</span>
              </span>
            </span>
            <span class="savings">{money(c.savings)} <span class="muted">savings</span></span>
            {#if c.professionLabel}<span class="profession">{c.professionLabel}</span>{/if}
            <span class="body muted">{c.height}, {c.weightKg} kg · {c.skin} skin · {c.hair} hair · {c.orientation}</span>
            {#if c.skills.length > 0}
              <span class="list">
                {#each c.skills as sk (sk.name)}
                  <span class:trade={sk.name === c.profession}>{sk.name} {sk.rank}/{sk.maxRank} <span class="muted">{sk.label}</span></span>
                {/each}
              </span>
            {:else}
              <span class="list muted">no skills yet</span>
            {/if}
            {#if c.traits.length > 0}
              <span class="list">
                {#each c.traits as tr (tr.name)}<span class="trait" title={tr.desc}>{tr.name}</span>{/each}
              </span>
            {/if}
          </label>
        </li>
      {/each}
    </ul>

    <dl>
      <dt>Passage</dt><dd>{money(cost)} <span class="muted">for {picks.length} at {money(t.cost)}</span></dd>
      <dt>They bring</dt><dd>{money(brought)}</dd>
    </dl>
    {#if blocked}<p class="warn">{blocked}</p>{/if}
    <div class="row">
      <button type="button" class="go" disabled={picks.length === 0 || blocked !== null} onclick={hire}>
        {picks.length === 0 ? 'Tick someone to hire' : `Hire ${picks.length} for ${money(cost)}`}
      </button>
      <button type="button" disabled={!canRoll} onclick={rollRecruits} title="Pay the recruiter again for a new set">
        New set for {money(t.fee)}
      </button>
      <button type="button" onclick={dismiss} title="Hire nobody from this set">Turn away</button>
    </div>
  {/if}
{/if}

<style>
  p { margin: 0 0 8px; }
  .muted { color: var(--muted); }
  .warn { color: #ffb36b; }
  .row { display: flex; flex-wrap: wrap; gap: 6px; margin: 8px 0; }
  .row button.go { background: var(--accent); color: #fff; }
  .row button.go:disabled { background: transparent; color: inherit; }
  dl { display: grid; grid-template-columns: max-content 1fr; gap: 3px 12px; margin: 8px 0; }
  dt { color: var(--muted); }
  dd { margin: 0; font-variant-numeric: tabular-nums; }
  .cards {
    list-style: none; padding: 0; margin: 0 0 8px;
    display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 6px;
  }
  .card {
    display: flex; flex-direction: column; gap: 4px; height: 100%; box-sizing: border-box;
    padding: 8px; border: 1px solid var(--line); border-radius: 6px; cursor: pointer; font-size: 12px;
  }
  .card.on { border-color: var(--accent); background: rgba(224, 112, 58, 0.12); }
  .card input { align-self: flex-end; margin: 0 0 -18px; }
  .head { display: flex; gap: 6px; align-items: center; }
  .glyph { font-size: 26px; line-height: 1; }
  .who { display: flex; flex-direction: column; font-size: 13px; }
  .savings { font-size: 14px; font-variant-numeric: tabular-nums; }
  .profession, .trade { color: #e9c46a; }
  .list { display: flex; flex-direction: column; }
  .trait { color: #e9c46a; }
</style>
