import type { ApiUsageDay } from '#lib/types.js';

/** The last 30 UTC days ending today, oldest first, with a zero for each day the server
 *  sent no row for, in the shape the bar chart draws. */
export function dailyUsage(rows: ApiUsageDay[], now: Date, days = 30): { date: string; new: number }[] {
  const byDay = new Map(rows.map((r) => [r.day, r.requests]));
  const end = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate());
  return Array.from({ length: days }, (_, i) => {
    const date = new Date(end - (days - 1 - i) * 86_400_000).toISOString().slice(0, 10);
    return { date, new: byDay.get(date) ?? 0 };
  });
}
