import { describe, expect, it } from 'vitest';
import { buddyFeedDays, buddyFeedPosts, buddyJobLines } from './buddyFeed';
import type { BuddyFeedItem } from './types';

const summary = { category: 'backend', seniority: 'senior', country: 'de' };
function item(id: number, handle: string, type: BuddyFeedItem['type'], day: string): BuddyFeedItem {
  return { id, handle, card_summary: summary, company: { slug: `c${id}`, name: `C${id}` }, job_title: null, job_slug: null, job_location: null, reactions: { counts: {}, mine: '' }, type, day };
}

describe('buddyFeedPosts', () => {
  it('folds one member’s applications on one day into a single post, in feed order', () => {
    const posts = buddyFeedPosts([
      item(9, 'amber-heron-42', 'applied', '2026-10-30'),
      item(8, 'tidy-kestrel-318', 'interview', '2026-10-30'),
      item(7, 'amber-heron-42', 'applied', '2026-10-30'),
      item(6, 'amber-heron-42', 'applied', '2026-10-29'),
    ]);
    expect(posts.map((p) => p.kind)).toEqual(['applied', 'event', 'event']);
    const first = posts[0];
    expect(first?.kind === 'applied' ? first.items.map((i) => i.id) : null).toEqual([9, 7]);
  });

  it('keeps a lone application, an interview and an offer as posts of their own', () => {
    const posts = buddyFeedPosts([
      item(3, 'a-b-1', 'offer', '2026-10-30'),
      item(2, 'a-b-1', 'interview', '2026-10-30'),
      item(1, 'a-b-1', 'applied', '2026-10-30'),
    ]);
    expect(posts.map((p) => p.kind)).toEqual(['event', 'event', 'event']);
  });

  it('gives every post a stable, unique key', () => {
    const posts = buddyFeedPosts([
      item(4, 'x-y-1', 'applied', '2026-10-30'),
      item(3, 'x-y-1', 'applied', '2026-10-30'),
      item(2, 'x-y-1', 'applied', '2026-10-29'),
    ]);
    expect(new Set(posts.map((p) => p.key)).size).toBe(posts.length);
  });
});

describe('buddyFeedDays', () => {
  it('groups posts under their day, newest day first as the feed reads', () => {
    const days = buddyFeedDays(
      buddyFeedPosts([
        item(3, 'a-b-1', 'offer', '2026-10-30'),
        item(2, 'c-d-1', 'interview', '2026-10-30'),
        item(1, 'c-d-1', 'interview', '2026-10-28'),
      ]),
    );
    expect(days.map((d) => [d.day, d.posts.length])).toEqual([
      ['2026-10-30', 2],
      ['2026-10-28', 1],
    ]);
  });
});

describe('buddyJobLines', () => {
  const company = { slug: 'cloudflare', name: 'Cloudflare' };
  it('titles a posting by its job and places it under the company', () => {
    expect(buddyJobLines({ company, job_title: 'Systems Engineer', job_location: 'Lisbon' })).toEqual({
      title: 'Systems Engineer',
      where: 'Cloudflare · Lisbon',
    });
  });
  it('names the company once when the posting is gone', () => {
    expect(buddyJobLines({ company, job_title: null, job_location: null })).toEqual({ title: 'Cloudflare', where: '' });
  });
});
