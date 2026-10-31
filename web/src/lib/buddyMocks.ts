import type { BuddyMock, BuddyMockHost, BuddyMockSummary, BuddyMockTopic } from './types.js';

/** A mock runs one hour: buddy.MockDuration on the server. */
export const MOCK_DURATION_MINUTES = 60;

/** The topics a host may offer, in the server's order. */
export const MOCK_TOPICS: readonly BuddyMockTopic[] = ['coding', 'system_design', 'behavioral'];

const TOPIC_LABELS: Record<BuddyMockTopic, string> = {
  coding: 'Coding',
  system_design: 'System design',
  behavioral: 'Behavioral',
};

export function mockTopicLabel(topic: BuddyMockTopic): string {
  return TOPIC_LABELS[topic];
}

/** The server refuses any link that is not https:// — saying so before the request. */
export function isHttpsLink(raw: string): boolean {
  const value = raw.trim();
  if (!value.startsWith('https://')) return false;
  try {
    return new URL(value).host !== '';
  } catch {
    return false;
  }
}

/** A booked mock's day and hours in `zone`, split for a date tile: "Tue" / "10" / "Nov"
 *  and "19:00–20:00". A mock carries absolute instants, so parsing them is safe here —
 *  unlike a slot label, which the server already resolved (web/AGENTS.md). */
export function mockDateParts(
  mock: Pick<BuddyMock, 'starts_at' | 'ends_at'>,
  zone: string,
): { weekday: string; day: string; month: string; time: string } {
  const start = new Date(mock.starts_at);
  const part = (opts: Intl.DateTimeFormatOptions) =>
    new Intl.DateTimeFormat('en-GB', { ...opts, timeZone: zone }).format(start);
  const time = new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit', hourCycle: 'h23', timeZone: zone });
  return {
    weekday: part({ weekday: 'short' }),
    day: part({ day: 'numeric' }),
    month: part({ month: 'short' }),
    time: `${time.format(start)}–${time.format(new Date(mock.ends_at))}`,
  };
}

/** The member page's one-line record. An average of no ratings is not zero stars. */
export function reviewSummaryLine(s: Pick<BuddyMockSummary, 'completed' | 'rating_count' | 'rating_avg'>): string {
  const mocks = s.completed === 1 ? '1 mock interview' : `${s.completed} mock interviews`;
  if (s.rating_count === 0) return `${mocks} · no ratings yet`;
  const ratings = s.rating_count === 1 ? '1 rating' : `${s.rating_count} ratings`;
  return `${mocks} · ${s.rating_avg.toFixed(1)} ★ from ${ratings}`;
}

/** The other participant by handle — or nothing about them once they left or a block
 *  stands between you, the rule the buddy conversation list keeps. */
export function mockCounterpartLabel(mock: Pick<BuddyMock, 'counterpart'>): string {
  return mock.counterpart ?? 'A former buddy';
}

export function mockRoleLabel(mock: Pick<BuddyMock, 'role'>): string {
  return mock.role === 'host' ? 'You host' : 'You are the guest';
}

const SHORT_DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
const SHORT_MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

/** A host's next slot as "Tue 4 Nov, 19:00", read from the slot's `local_start`, which the
 *  server already resolved in the viewer's zone. The wall clock is taken from the string
 *  itself: parsing it into a Date would re-read it in the browser's zone (web/AGENTS.md).
 *  The weekday comes from the calendar date alone. */
export function hostSlotLabel(localStart: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}:\d{2})/.exec(localStart);
  if (!m) return localStart;
  const [, y, mo, d, clock] = m;
  const weekday = new Date(Date.UTC(Number(y), Number(mo) - 1, Number(d))).getUTCDay();
  return `${SHORT_DAYS[weekday]} ${Number(d)} ${SHORT_MONTHS[Number(mo) - 1]}, ${clock}`;
}

/** A host's record in the listing, "4.6 · 5 mocks", or null before any revealed rating:
 *  an average of nothing is not a score, and a bare mock count reads as one. */
export function hostRatingLine(h: Pick<BuddyMockHost, 'rating_count' | 'rating_avg' | 'completed'>): string | null {
  if (h.rating_count === 0) return null;
  return `${h.rating_avg.toFixed(1)} · ${h.completed === 1 ? '1 mock' : `${h.completed} mocks`}`;
}
