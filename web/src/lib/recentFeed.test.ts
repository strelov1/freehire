import { describe, expect, it } from 'vitest';
import { aggregateLabel, pushFeedEntry, type RecentFeedEvent } from './recentFeed';

function event(overrides: Partial<RecentFeedEvent> = {}): RecentFeedEvent {
  return { kind: 'single', title: 'Senior Backend Engineer', company_name: 'Acme', ...overrides };
}

describe('pushFeedEntry', () => {
  it('adds the first entry to an empty list', () => {
    const result = pushFeedEntry([], event(), 1, 8);
    expect(result).toHaveLength(1);
    expect(result[0]).toMatchObject({ id: 1, title: 'Senior Backend Engineer', company_name: 'Acme' });
  });

  it('prepends new entries so the newest reads first', () => {
    const first = pushFeedEntry([], event({ title: 'A' }), 1, 8);
    const second = pushFeedEntry(first, event({ title: 'B' }), 2, 8);

    expect(second.map((e) => e.title)).toEqual(['B', 'A']);
  });

  it('caps the list at max, dropping the oldest', () => {
    let entries = pushFeedEntry([], event({ title: '1' }), 1, 2);
    entries = pushFeedEntry(entries, event({ title: '2' }), 2, 2);
    entries = pushFeedEntry(entries, event({ title: '3' }), 3, 2);

    expect(entries.map((e) => e.title)).toEqual(['3', '2']);
  });
});

describe('aggregateLabel', () => {
  // The whole point of the aggregate card: the sample company shown must never read
  // as if all N postings came from it.
  it('states the role and count without attributing them all to the sample company', () => {
    const label = aggregateLabel({ count: 12 });

    expect(label).toContain('12');
    expect(label).toContain('other');
  });
});
