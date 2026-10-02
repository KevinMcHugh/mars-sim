// The Charts tab's colors and the activity grouping. The page is dark-only,
// so these are the dark steps of the dataviz reference palette, checked with
// its validator against CHART_SURFACE (all adjacent pairs pass; the worst
// colorblind separation, yellow beside aqua, is ΔE 8.4 against an 8 target).
// See docs/frontend-web.md, "Charts".

export const CHART_SURFACE = '#1a1a19';
export const INK = '#ffffff';
export const INK_2 = '#c3c2b7';
export const MUTED = '#898781';
export const GRID = '#2c2c2a';
export const AXIS = '#383835';

/** The axis every chart shares: muted ink, hairline grid. */
export function axis(extra: Partial<import('uplot').Axis> = {}): import('uplot').Axis {
  return {
    stroke: MUTED, grid: { stroke: GRID, width: 1 }, ticks: { stroke: AXIS, width: 1, size: 4 },
    font: '11px system-ui, sans-serif', ...extra,
  };
}

/** Categorical slots, in their validated order. Never cycled past eight. */
export const SERIES = ['#3987e5', '#d95926', '#199e70', '#c98500', '#d55181', '#008300', '#9085e9', '#e66767'];

/** The idle band: a neutral remainder, not a categorical hue. */
export const IDLE = '#5a5955';

/**
 * The activity chart's bands, bottom to top, as the TUI stacks them: needs
 * at the floor, work in the middle, danger above it, idle as the lid. The
 * game has 14 activities and a chart gets at most 8 hues, so related ones
 * share a band; the tooltip and the legend still name each one.
 */
export const ACTIVITY_GROUPS: { label: string; color: string; activities: string[] }[] = [
  { label: 'sleeping', color: SERIES[0], activities: ['sleeping'] },
  { label: 'eating', color: SERIES[1], activities: ['eating'] },
  { label: 'relieving & washing', color: SERIES[2], activities: ['relieving', 'washing'] },
  { label: 'socializing', color: SERIES[3], activities: ['socializing'] },
  { label: 'cooking', color: SERIES[4], activities: ['cooking'] },
  { label: 'mining & building', color: SERIES[5], activities: ['mining', 'building'] },
  { label: 'hauling & cleaning', color: SERIES[6], activities: ['hauling', 'cleaning'] },
  { label: 'danger', color: SERIES[7], activities: ['escaping', 'fleeing', 'fighting'] },
  { label: 'idle', color: IDLE, activities: ['idle'] },
];

/** The color of an activity's band. */
export function activityColor(activity: string): string {
  return ACTIVITY_GROUPS.find((g) => g.activities.includes(activity))?.color ?? IDLE;
}

/**
 * The fill for walking to an activity: its color as 45° hatching over a
 * darker step of it, so walk time reads as the same band, textured, not as a
 * second hue (the dataviz texture channel).
 */
export function hatch(color: string): CanvasPattern | string {
  const size = 8;
  const c = document.createElement('canvas');
  c.width = c.height = size * devicePixelRatio;
  const g = c.getContext('2d');
  if (!g) return color;
  g.scale(devicePixelRatio, devicePixelRatio);
  g.fillStyle = color;
  g.fillRect(0, 0, size, size);
  g.fillStyle = 'rgba(26, 26, 25, 0.55)'; // CHART_SURFACE, part-way
  g.fillRect(0, 0, size, size);
  g.strokeStyle = color;
  g.lineWidth = 2;
  g.beginPath();
  for (const o of [-size, 0, size]) { g.moveTo(o, size); g.lineTo(o + size, 0); }
  g.stroke();
  return g.createPattern(c, 'repeat') ?? color;
}
