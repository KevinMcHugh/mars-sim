// Shared number formats for the panels.

// One formatter for every call: toLocaleString() builds its locale data
// anew each time, and the Market formats a few thousand amounts a send.
const grouped = new Intl.NumberFormat();

/** Dollars as the game writes them (sim.Money.String), with thousands separators. */
export function money(n: number): string {
  return (n < 0 ? '-$' : '$') + grouped.format(Math.abs(n));
}

const pad = (n: number) => String(n).padStart(2, '0');
const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;

/** A clock time from minutes since midnight (Stats.MinuteOfDay): "06:00". */
export function clock(minute: number): string {
  return `${pad(Math.floor(minute / 60))}:${pad(minute % 60)}`;
}

/** A day and clock time as the top bar reads them: "Day 12 12:43". */
export function dayClock(day: number, minute: number): string {
  return `Day ${grouped.format(day)} ${clock(minute)}`;
}

/** The colony calendar from the Hello: ticks a day, and the hour the landing (tick 0) fell. */
export interface Calendar { ticksPerDay: number; landingHour: number }

/**
 * The day and clock time a tick fell on, as sim.DayOf and sim.MinuteOfDay
 * compute them: the landing is day 1 at landingHour, and the day turns over
 * at midnight.
 */
export function dayMinute(tick: number, cal: Calendar): { day: number; minute: number } {
  const tpd = Math.max(cal.ticksPerDay, 1);
  const t = tick + Math.floor((tpd * cal.landingHour) / 24);
  return { day: Math.floor(t / tpd) + 1, minute: Math.floor(((t % tpd) * 1440) / tpd) };
}

/** When a tick fell, as the top bar reads the clock: "Day 3 07:05". Without a calendar (no Hello yet) it is the tick. */
export function when(tick: number, cal: Calendar | null | undefined): string {
  if (!cal) return `tick ${grouped.format(tick)}`;
  const { day, minute } = dayMinute(tick, cal);
  return dayClock(day, minute);
}

/**
 * A run of ticks from first to last: "Day 3 07:05–09:10", naming the second
 * day only when it differs ("Day 3 23:40–Day 4 00:20").
 */
export function whenRange(first: number, last: number, cal: Calendar | null | undefined): string {
  if (!cal) return `ticks ${grouped.format(first)}–${grouped.format(last)}`;
  const a = dayMinute(first, cal), b = dayMinute(last, cal);
  if (a.day === b.day && a.minute === b.minute) return dayClock(a.day, a.minute);
  return `${dayClock(a.day, a.minute)}–${a.day === b.day ? clock(b.minute) : dayClock(b.day, b.minute)}`;
}

/**
 * A stretch of colony time, in colony minutes (a day is 1440 however many
 * ticks it lasts), to its two largest units: "3 days, 2 hrs", "5 hrs, 12
 * min", "40 min". Under a minute is "under a minute".
 */
export function span(minutes: number): string {
  const m = Math.max(0, Math.floor(minutes));
  if (m < 1) return 'under a minute';
  const days = Math.floor(m / 1440), hrs = Math.floor((m % 1440) / 60), min = m % 60;
  if (days > 0) return hrs > 0 ? `${plural(days, 'day')}, ${plural(hrs, 'hr')}` : plural(days, 'day');
  if (hrs > 0) return min > 0 ? `${plural(hrs, 'hr')}, ${min} min` : plural(hrs, 'hr');
  return `${min} min`;
}
