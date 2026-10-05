// The Storage tab's arithmetic over the storage topic (StorageRow,
// internal/wire/boards.go): the colony's pooled stock, and the searches by
// item and by owner. Kept out of StoragePanel so node --test can run it.

export interface Holding { item: string; count: number }
export interface LedgerItem { owner: string; item: string; count: number }

export interface StorageRow {
  x: number; y: number; label: string;
  used: number; slots: number; items: number; capacity: number;
  top?: string;
  contents: Holding[];
  ledger: LedgerItem[];
}

export interface Pool {
  containers: number;
  used: number; slots: number;
  items: number; capacity: number;
  /** Every item across every container, most first, then by name. */
  totals: Holding[];
}

/** The colony's stock summed across every container. */
export function pool(rows: readonly StorageRow[]): Pool {
  const p: Pool = { containers: rows.length, used: 0, slots: 0, items: 0, capacity: 0, totals: [] };
  const by = new Map<string, number>();
  for (const r of rows) {
    p.used += r.used; p.slots += r.slots; p.items += r.items; p.capacity += r.capacity;
    for (const h of r.contents) by.set(h.item, (by.get(h.item) ?? 0) + h.count);
  }
  p.totals = [...by].map(([item, count]) => ({ item, count })).sort(mostFirst);
  return p;
}

/** Every owner on any ledger, by name: the owner search's suggestions. */
export function owners(rows: readonly StorageRow[]): string[] {
  return [...new Set(rows.flatMap((r) => r.ledger.map((l) => l.owner)))].sort();
}

/** Every item in any container, by name: the item search's suggestions. */
export function items(rows: readonly StorageRow[]): string[] {
  return [...new Set(rows.flatMap((r) => r.contents.map((h) => h.item)))].sort();
}

export interface Query { item: string; owner: string }

/** One container a search found, with the lines that matched. */
export interface Hit {
  row: StorageRow;
  /** Ledger lines (owner set) for an owner search, else content totals. */
  lines: { owner?: string; item: string; count: number }[];
  total: number;
}

export interface Found {
  hits: Hit[];
  total: number;
  /** The matched items summed across the hits, most first. */
  totals: Holding[];
}

/**
 * The containers holding what a query asks for, most first. Both terms are
 * case-insensitive substrings and either may be empty; null when both are.
 * An item-only search reads the contents, so it finds stock nobody owns as
 * well. Any owner term reads the ledger instead, since only the ledger says
 * whose a stack is; "Uma" also finds what Uma has listed "for sale by Uma".
 */
export function search(rows: readonly StorageRow[], q: Query): Found | null {
  const item = q.item.trim().toLowerCase();
  const owner = q.owner.trim().toLowerCase();
  if (!item && !owner) return null;
  const has = (s: string, term: string) => s.toLowerCase().includes(term);
  const hits: Hit[] = [];
  const by = new Map<string, number>();
  for (const row of rows) {
    const lines = owner
      ? row.ledger.filter((l) => has(l.owner, owner) && has(l.item, item))
      : row.contents.filter((h) => has(h.item, item));
    if (lines.length === 0) continue;
    let total = 0;
    for (const l of lines) {
      total += l.count;
      by.set(l.item, (by.get(l.item) ?? 0) + l.count);
    }
    hits.push({ row, lines, total });
  }
  hits.sort((a, b) => b.total - a.total || a.row.label.localeCompare(b.row.label) || a.row.y - b.row.y || a.row.x - b.row.x);
  const totals = [...by].map(([item, count]) => ({ item, count })).sort(mostFirst);
  return { hits, total: totals.reduce((n, h) => n + h.count, 0), totals };
}

/**
 * A hit's lines as one line of text, each owner named once:
 * "Uma Xu: iron ore ×30, meal ×2 · the colony: meal ×20".
 */
export function describe(lines: Hit['lines']): string {
  const by = new Map<string | undefined, string[]>();
  for (const l of lines) {
    const g = by.get(l.owner) ?? [];
    g.push(`${l.item} ×${l.count}`);
    by.set(l.owner, g);
  }
  return [...by].map(([owner, items]) => (owner ? `${owner}: ` : '') + items.join(', ')).join(' · ');
}

function mostFirst(a: Holding, b: Holding): number {
  return b.count - a.count || a.item.localeCompare(b.item);
}
