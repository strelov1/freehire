import { describe, it, expect } from 'vitest';
import { CATEGORY_VALUES } from './generated/contracts';
import { CATEGORY_LABELS, RELOCATION_LABELS, categoryLabel, titleCase } from './labels';

describe('CATEGORY_LABELS', () => {
  // The category map is the one label map that must be exhaustive: three surfaces
  // render it through two different fallbacks (facets.ts title-cases every word,
  // enrichment.ts capitalizes only the first), so any code the map omits renders two
  // ways by construction. This test is what keeps the map bound to the backend
  // vocabulary — a new category fails here instead of quietly rendering twice.
  it('labels every category in the generated vocabulary', () => {
    const unlabelled = CATEGORY_VALUES.filter((value) => !(value in CATEGORY_LABELS));
    expect(unlabelled).toEqual([]);
  });

  it('names AI engineering as a discipline, not a job title', () => {
    expect(CATEGORY_LABELS.ai_engineering).toBe('AI Engineering');
  });

  it('spells fullstack the way the indexed insights page already does', () => {
    expect(CATEGORY_LABELS.fullstack).toBe('Full-Stack');
  });
});

describe('categoryLabel', () => {
  it('reads the shared map', () => {
    expect(categoryLabel('ml_ai')).toBe('ML / AI');
  });

  // The safety net for a vocabulary the SPA has not been taught yet — not the
  // mechanism by which known categories are labelled, which is the map above.
  it('title-cases a code it has never seen', () => {
    expect(categoryLabel('quantum_widgets')).toBe('Quantum Widgets');
  });
});

describe('titleCase', () => {
  it('capitalizes every word of a snake_case code', () => {
    expect(titleCase('network_engineering')).toBe('Network Engineering');
  });
});

describe('RELOCATION_LABELS', () => {
  // The filter panel said "None" and the job page said "Not supported" for the same
  // code. "None" reads as "not stated"; the other two values of this facet are
  // participles, so the negative one is too.
  it('reads as a participle, parallel to its sibling values', () => {
    expect(RELOCATION_LABELS.not_supported).toBe('Not supported');
    expect(RELOCATION_LABELS.supported).toBe('Supported');
    expect(RELOCATION_LABELS.required).toBe('Required');
  });
});
