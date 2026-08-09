import { describe, expect, it } from 'vitest';
import { RAIL } from './filterSections';

describe('the ai_archetype rail entry', () => {
  it('is registered right after category, so the facet is reachable in the modal', () => {
    const categoryIndex = RAIL.findIndex((e) => e.key === 'category');
    const archetypeIndex = RAIL.findIndex((e) => e.key === 'ai_archetype');
    expect(archetypeIndex).toBeGreaterThan(-1);
    expect(archetypeIndex).toBe(categoryIndex + 1);
  });

  it('renders through the generic facet path with the ai_archetype param', () => {
    const entry = RAIL.find((e) => e.key === 'ai_archetype');
    expect(entry?.kind).toBe('facet');
    expect(entry?.facetParam).toBe('ai_archetype');
  });
});
