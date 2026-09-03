import { describe, expect, it } from 'vitest';
import type { Reality } from './generated/contracts';
import { cardFreshnessBadges, daysSince, freshnessBadges } from './freshness';

const daysAgo = (n: number) => new Date(Date.now() - n * 86_400_000).toISOString();

const reality = (over: Partial<Reality> = {}): Reality => ({
  class: 'fresh',
  age_days: 1,
  repost_count: 1,
  mass_posting_count: 1,
  fake_freshness: false,
  ...over,
});

describe('daysSince', () => {
  it('counts whole days', () => {
    expect(daysSince(daysAgo(3))).toBe(3);
  });

  it('is null for a missing or unparseable date', () => {
    expect(daysSince(null)).toBeNull();
    expect(daysSince('not a date')).toBeNull();
  });

  it('clamps a source clock running ahead to zero rather than reporting the future', () => {
    expect(daysSince(new Date(Date.now() + 6 * 3_600_000).toISOString())).toBe(0);
  });
});

describe('freshnessBadges', () => {
  it('marks a today-posted job new and invites an early applicant', () => {
    expect(freshnessBadges(daysAgo(0), reality(), 0).map((b) => b.label)).toEqual([
      'New',
      'Be an early applicant',
    ]);
  });

  it('keeps New past the early window but drops the invitation', () => {
    expect(freshnessBadges(daysAgo(5), reality(), 0).map((b) => b.label)).toEqual(['New']);
  });

  it('drops the invitation once enough people have said they applied', () => {
    expect(freshnessBadges(daysAgo(1), reality(), 9).map((b) => b.label)).toEqual(['New']);
  });

  it('says nothing after a week', () => {
    expect(freshnessBadges(daysAgo(30), reality(), 0)).toEqual([]);
  });

  // The reason this module reads `reality` at all: a source that rewrites its posting
  // date every crawl would otherwise stamp "New" on the oldest job in the catalogue.
  it('says nothing when the reality signal distrusts the posting date', () => {
    expect(freshnessBadges(daysAgo(0), reality({ fake_freshness: true }), 0)).toEqual([]);
  });

  it('says nothing when the job has been open long enough to be classified stale', () => {
    expect(freshnessBadges(daysAgo(0), reality({ class: 'stale', age_days: 60 }), 0)).toEqual([]);
  });

  // A projection without counts carries no reality at all; the date then stands alone.
  it('trusts the date when no reality signal was computed', () => {
    expect(freshnessBadges(daysAgo(1), null, 0).map((b) => b.label)).toEqual([
      'New',
      'Be an early applicant',
    ]);
  });

  it('says nothing without a posting date', () => {
    expect(freshnessBadges(null, reality(), 0)).toEqual([]);
  });

  it('states what the applied count actually counted', () => {
    const [, early] = freshnessBadges(daysAgo(1), reality(), 2);
    expect(early?.tooltip).toContain('2 people have told us they applied');
  });
});

// The card's gate. Unlike the detail page, a card can be drawn from a projection that
// carries no reality signal at all (the /jobs list does not attach it; the tracking and
// assistant Card shape has no such field), and there the date would stand alone on
// exactly the postings the fake-freshness gate exists to suppress.
describe('cardFreshnessBadges', () => {
  it('shows the pair when the signal is present and fresh', () => {
    expect(cardFreshnessBadges(daysAgo(1), reality(), 0).map((b) => b.label)).toEqual([
      'New',
      'Be an early applicant',
    ]);
  });

  it('shows nothing when no reality signal was computed, however fresh the date', () => {
    expect(cardFreshnessBadges(daysAgo(0), null, 0)).toEqual([]);
    expect(cardFreshnessBadges(daysAgo(0), undefined, 0)).toEqual([]);
  });

  it('still defers to the signal when it distrusts the date', () => {
    expect(cardFreshnessBadges(daysAgo(0), reality({ fake_freshness: true }), 0)).toEqual([]);
  });

  it('still applies the shared thresholds', () => {
    expect(cardFreshnessBadges(daysAgo(5), reality(), 0).map((b) => b.label)).toEqual(['New']);
    expect(cardFreshnessBadges(daysAgo(20), reality(), 0)).toEqual([]);
  });

  // The wrapper must not quietly relax the applied bound the shared rule enforces.
  it('still applies the shared applied-count bound', () => {
    expect(cardFreshnessBadges(daysAgo(1), reality(), 9).map((b) => b.label)).toEqual(['New']);
  });
});

// A closed posting is never worth hurrying to. The rule used to live in two templates —
// `{#if !job.closed_at}` on the detail page and a ternary on the card — which is exactly
// the drift the shared rule exists to prevent, one layer up from where it was caught.
describe('closed postings', () => {
  it('earn no badge however fresh the date', () => {
    expect(freshnessBadges(daysAgo(0), reality(), 0, daysAgo(0))).toEqual([]);
    expect(cardFreshnessBadges(daysAgo(0), reality(), 0, daysAgo(0))).toEqual([]);
  });

  it('leave an open posting badged, whether the close date is null or absent', () => {
    expect(freshnessBadges(daysAgo(1), reality(), 0, null).map((b) => b.label)).toEqual([
      'New',
      'Be an early applicant',
    ]);
    expect(cardFreshnessBadges(daysAgo(1), reality(), 0).map((b) => b.label)).toEqual([
      'New',
      'Be an early applicant',
    ]);
  });
});
