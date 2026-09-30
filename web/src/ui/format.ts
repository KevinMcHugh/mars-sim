// Shared number formats for the panels.

/** Dollars as the game writes them (sim.Money.String), with thousands separators. */
export function money(n: number): string {
  return (n < 0 ? '-$' : '$') + Math.abs(n).toLocaleString();
}
