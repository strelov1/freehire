import { talentHeading } from '#lib/talentCard.js';

// How an interview buddy is PRESENTED — the heading, the day of an event, the signals two
// members share. Pure and `$app`-free, so every surface showing a member (feed, similar
// list, conversation rows, the member page) words them the same way.

export const BUDDY_HEADING_FALLBACK = 'Member';

/** A member's heading: the same dictionary heading the Talent Network shows ("Senior
 *  Backend"), never the handle. A member whose card resolves neither value reads as
 *  "Member" — the handle still sits beside it. */
export function buddyHeading(summary: { seniority?: string; category?: string }): string {
  return talentHeading(summary.seniority || undefined, summary.category || undefined, BUDDY_HEADING_FALLBACK);
}

/** Today's calendar day in the viewer's own zone, as YYYY-MM-DD. */
export function localDay(now: Date = new Date()): string {
  const m = String(now.getMonth() + 1).padStart(2, '0');
  const d = String(now.getDate()).padStart(2, '0');
  return `${now.getFullYear()}-${m}-${d}`;
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

// Beyond two weeks "23 days ago" is harder to place than the date itself.
const RELATIVE_DAYS = 14;

function dayNumber(day: string): number | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(day);
  if (!match) return null;
  return Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3])) / 86_400_000;
}

/** When a feed event happened, relative to `today` (both YYYY-MM-DD).
 *
 *  The API carries only the DAY, never a time, so the answer is a day too: "today",
 *  "yesterday", "3 days ago", then the date itself ("Oct 3", with the year when it is not
 *  this year's). Integer day arithmetic — no `Date` is parsed from the string, which would
 *  re-read the day in the browser's zone and move it. A day after `today` (the server's
 *  day running ahead of the viewer's zone) reads as "today". */
export function buddyRelativeDay(day: string, today: string): string {
  const then = dayNumber(day);
  const now = dayNumber(today);
  if (then === null || now === null) return day;
  const diff = now - then;
  if (diff <= 0) return 'today';
  if (diff === 1) return 'yesterday';
  if (diff <= RELATIVE_DAYS) return `${diff} days ago`;
  const year = day.slice(0, 4);
  const label = `${MONTHS[Number(day.slice(5, 7)) - 1]} ${Number(day.slice(8, 10))}`;
  return year === today.slice(0, 4) ? label : `${label}, ${year}`;
}

/** What a similar member shares with the viewer, as a phrase.
 *
 *  Only the shared companies can be stated. The payload's score also counts a shared
 *  category and geography, but it cannot be decomposed — the category weight equals a
 *  shared country plus its region — and the viewer's own summary is not in the payload,
 *  so "same role" here would be a guess. */
export function buddySharedCompanies(count: number): string {
  if (count <= 0) return '';
  return count === 1 ? '1 company in common' : `${count} companies in common`;
}
