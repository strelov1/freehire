import { describe, it, expect } from 'vitest';
import { collectionBySlug, collectionSlugs } from './collections';

describe('collectionBySlug', () => {
  it('resolves a company-membership collection to a collections facet param', () => {
    const c = collectionBySlug('yc');
    expect(c).toBeDefined();
    expect(c?.title).toBe('Y Combinator');
    expect(c?.params).toEqual({ collections: 'yc' });
  });

  it('resolves a filter collection to its own facet params', () => {
    const c = collectionBySlug('remote-worldwide');
    expect(c).toBeDefined();
    expect(c?.title).toBe('Remote Worldwide');
    expect(c?.params).toEqual({ work_mode: 'remote', regions: 'global' });
  });

  it('returns undefined for an unknown slug', () => {
    expect(collectionBySlug('does-not-exist')).toBeUndefined();
  });
});

describe('collectionSlugs', () => {
  it('lists every collection slug across both registries with no duplicates', () => {
    const slugs = collectionSlugs();
    expect(slugs).toContain('yc'); // company collection
    expect(slugs).toContain('remote-worldwide'); // filter collection
    expect(new Set(slugs).size).toBe(slugs.length);
  });
});
