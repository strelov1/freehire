import type { BuddyMock, BuddyMockSummary, BuddyMockTopic } from './types.js';

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

/** A booked mock's time in `zone`. A mock carries absolute instants, so parsing them is
 *  safe here — unlike a slot label, which the server already resolved (web/AGENTS.md). */
export function mockWhen(mock: Pick<BuddyMock, 'starts_at' | 'ends_at'>, zone: string): string {
  const clock = { hour: '2-digit', minute: '2-digit', hourCycle: 'h23', timeZone: zone } as const;
  const day = new Intl.DateTimeFormat('en-GB', { weekday: 'long', day: 'numeric', month: 'long', timeZone: zone });
  const time = new Intl.DateTimeFormat('en-GB', clock);
  const start = new Date(mock.starts_at);
  return `${day.format(start)}, ${time.format(start)}–${time.format(new Date(mock.ends_at))}`;
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
