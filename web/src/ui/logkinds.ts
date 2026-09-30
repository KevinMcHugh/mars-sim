// A color per colony-log type (sim.LogKind's labels), for the Log tab's type
// column and the map's ticker: how a reader spots a death without reading
// every line. A type not listed here reads in the muted color.

const colors: Record<string, string> = {
  death: '#f2877e',
  combat: '#f0a24a',
  'build start': '#8fb7d9',
  'build complete': '#7fcf96',
  arrival: '#8fd0ff',
  mutation: '#c99af2',
  haul: '#b8a58f',
  burn: '#e0703a',
  mate: '#f29ac4',
  birth: '#f2c1dc',
  escape: '#e9c46a',
  cavern: '#b2c7a0',
  nest: '#e05a8a',
};

export function logKindColor(kind: string): string {
  return colors[kind] ?? 'var(--muted)';
}
