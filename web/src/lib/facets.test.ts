import { describe, expect, it } from 'vitest';
import { COLLECTIONS } from './generated/contracts';
import { FACETS } from './facets';

const collectionOptions = () => FACETS.find((f) => f.param === 'collections')?.options ?? [];

describe('the collections facet', () => {
  it('offers every collection in the registry as a filter option', () => {
    // The options were built from `kind === 'editorial'` plus `kind === 'credential'`.
    // The day a third kind appeared, its collections silently vanished from the
    // filters — filterable in the API, unreachable in the UI.
    const registry = COLLECTIONS.map((c) => c.slug).sort();
    const offered = collectionOptions()
      .map((o) => o.value)
      .sort();
    expect(offered).toEqual(registry);
  });

  it('offers the backer collections', () => {
    const offered = collectionOptions().map((o) => o.value);
    for (const slug of ['yc', 'techstars', 'a16z-portfolio', 'a16z-speedrun']) {
      expect(offered).toContain(slug);
    }
  });

  it('carries the brand mark on a backer collection, and none on the others', () => {
    // The chip is how most readers meet these collections; the orange Y is
    // recognised well before the words beside it are read.
    const byValue = new Map(collectionOptions().map((o) => [o.value, o]));
    expect(byValue.get('yc')?.icon).toMatch(/\.png$/);
    expect(byValue.get('a16z-speedrun')?.icon).toMatch(/\.png$/);
    expect(byValue.get('bigtech')?.icon).toBeUndefined();
    expect(byValue.get('uk-skilled-worker-sponsor')?.icon).toBeUndefined();
  });

  it('groups credentials under their own heading, and leaves the rest ungrouped', () => {
    // A licence from a public register is not one of our curated picks; running the
    // two together would read as though we vouched for both the same way.
    for (const option of collectionOptions()) {
      const kind = COLLECTIONS.find((c) => c.slug === option.value)?.kind;
      expect(option.group).toBe(kind === 'credential' ? 'Employer credentials' : undefined);
    }
  });
});
