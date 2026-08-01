import { describe, expect, it } from 'vitest';
import { COLLECTIONS } from './generated/contracts';
import { backerBadges } from './backers';

describe('backerBadges', () => {
  it('surfaces a backer tag as its brand mark', () => {
    const badges = backerBadges(['yc']);
    expect(badges).toHaveLength(1);
    expect(badges[0]?.label).toBe('Y Combinator');
    expect(badges[0]?.mark).toMatch(/\.png$/);
  });

  it('states that the backing is a fact about the company, not the role', () => {
    // A fund picking the company says nothing about this posting's pay, quality or
    // hiring intent. The accessible name is where that distinction has to live,
    // because it is the only copy a screen-reader user gets.
    for (const badge of backerBadges(['yc', 'techstars', 'a16z-portfolio', 'a16z-speedrun'])) {
      expect(badge.alt).toMatch(/backed by|backer/i);
      expect(badge.alt).toContain(badge.label);
    }
  });

  it('carries the slug so a caller can route to the collection landing', () => {
    // The route is built by the component through resolve(), not spelled out here —
    // a hand-written path bypasses SvelteKit's typed routing (and the lint rule that
    // enforces it).
    expect(backerBadges(['a16z-speedrun'])[0]?.slug).toBe('a16z-speedrun');
  });

  it('ignores editorial collections and credentials', () => {
    expect(backerBadges(['bigtech', 'unicorn', 'uk-skilled-worker-sponsor'])).toEqual([]);
  });

  it('renders nothing for an unknown tag rather than inventing a mark', () => {
    expect(backerBadges(['not-a-collection', ''])).toEqual([]);
  });

  it('handles an absent tag list', () => {
    expect(backerBadges(undefined)).toEqual([]);
    expect(backerBadges([])).toEqual([]);
  });

  it('keeps registry order when a company holds several backers', () => {
    const both = backerBadges(['a16z-portfolio', 'yc']);
    expect(both.map((b) => b.slug)).toEqual(['yc', 'a16z-portfolio']);
  });

  it('covers every backer in the generated registry', () => {
    // A backer added to the Go registry with no mark here would tag jobs and filter
    // them while rendering nothing — this is what catches that.
    const backers = COLLECTIONS.filter((c) => c.kind === 'backer').map((c) => c.slug);
    expect(backerBadges(backers).map((b) => b.slug)).toEqual(backers);
  });
});
