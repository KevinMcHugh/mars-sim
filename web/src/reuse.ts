// Structural sharing for topic payloads (TopicData.set in game.svelte.ts).

/**
 * `next` with every part deep-equal to its counterpart in `prev` (same key, or
 * same index in an array) swapped for prev's object. A topic is resent whole
 * when anything in it changes, so without this every row of every list is a
 * new object on every update, and Svelte re-evaluates all of them: the
 * Market's few hundred colony orders, twice a second, for the handful that
 * changed. With it, a keyed {#each} skips the rows whose object is the same.
 * Mutates `next` (a fresh payload nothing else holds); returns prev if all of
 * it is equal.
 */
export function reuse(prev: unknown, next: unknown): unknown {
  if (prev === next || typeof prev !== 'object' || typeof next !== 'object' || prev === null || next === null) {
    return prev === next ? prev : next;
  }
  if (Array.isArray(next) !== Array.isArray(prev)) return next;
  const p = prev as Record<string, unknown>;
  const n = next as Record<string, unknown>;
  const keys = Object.keys(n);
  let same = keys.length === Object.keys(p).length;
  for (const k of keys) {
    const v = reuse(p[k], n[k]);
    n[k] = v;
    if (v !== p[k]) same = false;
  }
  return same ? prev : next;
}
