<script lang="ts">
  // Pause · Fast · Faster · Max. + and - step through it; space toggles pause.
  import { ui, setSpeed } from '../game.svelte';
  import { SPEEDS } from '../speed';
</script>

<div class="speed" role="radiogroup" aria-label="Speed">
  {#each SPEEDS as s, i (s.name)}
    <button
      type="button"
      role="radio"
      aria-checked={ui.speed === i}
      class:on={ui.speed === i}
      title={s.title}
      disabled={!ui.hello}
      onclick={() => setSpeed(i)}>{s.name}</button>
  {/each}
</div>

<style>
  .speed { display: flex; border: 1px solid var(--line); border-radius: 6px; overflow: hidden; }
  button { border: 0; border-radius: 0; padding: 4px 10px; background: transparent; }
  button + button { border-left: 1px solid var(--line); }
  button.on { background: var(--accent); color: #fff; }
</style>
