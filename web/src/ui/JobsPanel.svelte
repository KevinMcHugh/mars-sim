<script lang="ts">
  // The Jobs tab: the job board (the jobs topic, internal/wire/boards.go), as
  // the TUI's shows it. Selecting a project lays its tiles over the map:
  // yellow queued, orange being built, green done.
  import { centerOn, highlight, inspect, subscribe, topics, ui } from '../game.svelte';
  import { JOB_BUILDING, JOB_DONE, JOB_QUEUED } from '../map/palette';

  interface Person { id: number; name: string }
  interface Task { x: number; y: number; level: number; terrain: string; phase: number; done: boolean; builder?: Person }
  interface Project { id: number; name: string; queuedTick: number; done: number; tasks: Task[]; assignees: Person[] }
  interface Jobs { projects: Project[]; pending: Record<string, number> }

  $effect(() => subscribe('jobs'));
  const jobs = $derived(topics.data.jobs as Jobs | undefined);

  let openId: number | null = $state(null);
  const open = $derived(jobs?.projects.find((p) => p.id === openId));

  // The open project's tiles on the map, redrawn as its tasks change; gone
  // when it closes, finishes, or the tab unmounts.
  $effect(() => {
    // Only the shown level's tasks: the tint is drawn on the map as it is.
    highlight(open ? open.tasks.filter((t) => t.level === ui.level).map((t) => ({
      x: t.x, y: t.y, color: t.done ? JOB_DONE : t.builder ? JOB_BUILDING : JOB_QUEUED,
    })) : null);
  });
  $effect(() => () => highlight(null));

  function center(p: Project): void {
    const todo = p.tasks.filter((t) => !t.done);
    let ts = todo.length ? todo : p.tasks;
    if (!ts.length) return;
    // A project can span levels (a stair, a shaft): center on its first
    // task's level, and on that level's tasks.
    const level = ts[0].level;
    ts = ts.filter((t) => t.level === level);
    centerOn(Math.round(ts.reduce((s, t) => s + t.x, 0) / ts.length), Math.round(ts.reduce((s, t) => s + t.y, 0) / ts.length), level);
  }

  const pending = $derived(jobs ? Object.entries(jobs.pending) : []);
</script>

{#if !jobs}
  <p class="muted">Loading…</p>
{:else}
  <h2>Job board ({jobs.projects.length})</h2>
  {#if pending.length > 0}
    <p class="muted">
      Waiting for a build site: {pending.map(([what, n]) => `${n} ${what}${n === 1 ? '' : ' orders'}`).join(', ')}.
    </p>
  {/if}
  {#if jobs.projects.length === 0}
    <p class="muted">No jobs queued.</p>
  {/if}
  <ul class="projects">
    {#each jobs.projects as p (p.id)}
      <li class:on={p.id === openId}>
        <button type="button" class="head" onclick={() => (openId = openId === p.id ? null : p.id)} aria-expanded={p.id === openId}>
          <span class="name">{p.name}</span>
          <span class="frac">{p.done}/{p.tasks.length}</span>
          <span class="bar"><span style="width: {p.tasks.length ? (p.done * 100) / p.tasks.length : 0}%"></span></span>
          <span class="sub">{p.assignees.length} assigned · queued t{p.queuedTick}, {Math.max(0, ui.tick - p.queuedTick).toLocaleString()} ticks ago</span>
        </button>
        {#if p.id === openId}
          <div class="detail">
            <div class="row">
              <span class="muted">Assigned:</span>
              {#if p.assignees.length === 0}<span class="muted">nobody yet</span>{/if}
              {#each p.assignees as a, i (a.id)}{i > 0 ? ', ' : ''}<button type="button" class="link" onclick={() => inspect({ entity: a.id }, 'jobs')}>{a.name}</button>{/each}
              <button type="button" class="small find" onclick={() => center(p)}>Find</button>
            </div>
            <ul class="tasks">
              {#each p.tasks as t, i (i)}
                <li class:done={t.done}>
                  <button type="button" class="link coord" onclick={() => inspect({ tile: [t.x, t.y, t.level] }, 'jobs')}>({t.x}, {t.y}{t.level !== ui.hello?.landingLevel ? `, level ${t.level}` : ''})</button>
                  {t.terrain} —
                  {#if t.done}done{:else if t.builder}building: <button type="button" class="link" onclick={() => inspect({ entity: t.builder!.id }, 'jobs')}>{t.builder.name}</button>{:else}queued{/if}
                </li>
              {/each}
            </ul>
          </div>
        {/if}
      </li>
    {/each}
  </ul>
{/if}

<style>
  h2 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 4px 0 8px; }
  p { margin: 0 0 8px; line-height: 1.45; }
  .muted { color: var(--muted); }
  ul { list-style: none; padding: 0; margin: 0; }
  .projects { display: grid; gap: 4px; }
  .projects > li { border: 1px solid transparent; border-radius: 6px; }
  .projects > li.on { border-color: var(--accent); background: rgba(224, 112, 58, 0.1); }
  .head {
    width: 100%; display: grid; grid-template-columns: 1fr auto; gap: 3px 8px; text-align: left;
    border-color: transparent; background: transparent; padding: 6px 8px;
  }
  .head:hover { background: rgba(255, 255, 255, 0.05); }
  .name { font-weight: 600; }
  .frac { font-variant-numeric: tabular-nums; color: var(--muted); }
  .bar { grid-column: 1 / -1; height: 5px; border-radius: 3px; background: rgba(255, 255, 255, 0.08); overflow: hidden; }
  .bar span { display: block; height: 100%; background: #5fd38d; }
  .sub { grid-column: 1 / -1; color: var(--muted); font-size: 12px; }
  .detail { padding: 2px 8px 8px; }
  .row { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px; margin-bottom: 6px; }
  .find { margin-left: auto; }
  .tasks li { font-size: 12px; line-height: 1.6; }
  .tasks li.done { color: #7fcf96; }
  .coord { font-variant-numeric: tabular-nums; }
  .link { border: none; background: none; padding: 0; color: #8fd0ff; cursor: pointer; font: inherit; }
  .link:hover { text-decoration: underline; }
  .small { padding: 2px 8px; font-size: 12px; }
</style>
