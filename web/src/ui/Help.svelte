<script lang="ts">
  // A (?) that shows or hides an explanation. The explanation is the caller's
  // own markup, shown while `open` ({#if open}<p class="help-text">…{/if}), so
  // the button can sit in a heading and the text under it. Shut by default:
  // the tabs used to open with a paragraph each that a player reads once.
  // `float` puts it at the top right of a tab with no heading to sit in.
  let { open = $bindable(false), float = false, label = 'What is this?' }: {
    open?: boolean;
    float?: boolean;
    label?: string;
  } = $props();
</script>

<button type="button" class="help" class:float class:on={open} aria-expanded={open} aria-label={label} title={open ? 'Hide the explanation' : label}
  onclick={() => (open = !open)}>?</button>

<style>
  .help {
    display: inline-flex; align-items: center; justify-content: center;
    width: 16px; height: 16px; padding: 0; margin-left: 6px; vertical-align: 1px;
    border: 1px solid var(--muted); border-radius: 50%; background: transparent;
    color: var(--muted); font: 600 10px/1 system-ui, sans-serif; text-transform: none; letter-spacing: 0;
  }
  .help:hover, .help.on { color: var(--fg); border-color: var(--fg); }
  .help.on { background: rgba(255, 255, 255, 0.12); }
  .float { float: right; margin: 2px 0 4px 8px; }
</style>
