import { describe, expect, it } from 'vitest';
import { COLLECTIONS } from './collections';
import { FILTER_COLLECTIONS } from './filterCollections';
import { COLLECTION_SECTIONS, sectionBySlug } from './collectionSections';

describe('COLLECTION_SECTIONS', () => {
  // A collection whose group or kind names no section is dropped from the hub
  // without a word, so every one of them must land somewhere.
  it('gives every collection a section', () => {
    const keys = new Set<string>(COLLECTION_SECTIONS.map((s) => s.key));
    for (const fc of FILTER_COLLECTIONS) expect(keys, fc.slug).toContain(fc.group);
    for (const c of COLLECTIONS) expect(keys, c.slug).toContain(c.kind);
  });

  it('has unique keys and slugs', () => {
    expect(new Set(COLLECTION_SECTIONS.map((s) => s.key)).size).toBe(COLLECTION_SECTIONS.length);
    expect(new Set(COLLECTION_SECTIONS.map((s) => s.slug)).size).toBe(COLLECTION_SECTIONS.length);
  });

  it('resolves a section by its slug', () => {
    expect(sectionBySlug('visa-sponsors')?.key).toBe('credential');
    expect(sectionBySlug('nope')).toBeUndefined();
  });
});
