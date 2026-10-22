// Display rules for the public /semantic-search page. Every figure the API sends may be
// null — not yet measured — and a null must reach the reader as "measuring", never as
// 0: a zero on a public page is read as a measurement.

/** What an unmeasured figure renders as. */
export const MEASURING = 'measuring…';

const countFormat = new Intl.NumberFormat('en-US');

export function formatCount(n: number | null): string {
  return n === null ? MEASURING : countFormat.format(n);
}

/** The covered share of eligible postings, to one decimal, or null when either side is
 *  unmeasured or nothing is eligible. Rounded DOWN: 99.96% must not read as 100% while
 *  postings are still waiting. */
export function coveredPercent(covered: number | null, eligible: number | null): number | null {
  if (covered === null || eligible === null || eligible <= 0) return null;
  return Math.floor((covered / eligible) * 1000) / 10;
}

export function formatRate(perMinute: number | null): string {
  if (perMinute === null) return MEASURING;
  if (perMinute > 0 && perMinute < 1) return '<1 / min';
  return `${Math.round(perMinute)} / min`;
}

/** Time left at the current rate. An empty queue is "nothing left" whatever the rate
 *  says — a drained queue reading "<1 min" suggests work is still pending. */
export function formatEta(seconds: number | null, queued: number | null = null): string {
  if (queued === 0) return 'nothing left';
  if (seconds === null) return MEASURING;
  const minutes = Math.round(seconds / 60);
  if (minutes < 1) return '<1 min';
  if (minutes < 60) return `~${minutes} min`;
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  return rest === 0 ? `~${hours} h` : `~${hours} h ${rest} min`;
}

/** A measurement time as "HH:MM UTC". UTC, not the reader's zone, so the server
 *  render and the hydrated page print the same string. */
export function formatClock(iso: string | null): string {
  if (iso === null) return MEASURING;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return MEASURING;
  return `${String(d.getUTCHours()).padStart(2, '0')}:${String(d.getUTCMinutes()).padStart(2, '0')} UTC`;
}
