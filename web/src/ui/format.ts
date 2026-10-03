// Shared number formats for the panels.

// One formatter for every call: toLocaleString() builds its locale data
// anew each time, and the Market formats a few thousand amounts a send.
const grouped = new Intl.NumberFormat();

/** Dollars as the game writes them (sim.Money.String), with thousands separators. */
export function money(n: number): string {
  return (n < 0 ? '-$' : '$') + grouped.format(Math.abs(n));
}
