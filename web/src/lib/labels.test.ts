import { describe, it, expect } from 'vitest';
import { CATEGORY_LABELS, RELOCATION_LABELS, categoryLabel, titleCase } from './labels';

// Exhaustiveness over the generated vocabulary is enforced by the type checker rather
// than here: CATEGORY_LABELS carries `satisfies Record<Category, string>`, so
// `pnpm run check` fails in BOTH directions — a category added to the backend and left
// unlabelled (TS1360) and a label left behind after a category is removed (TS2353).
// These cases pin the wordings that were product decisions rather than mechanical ones.
describe('CATEGORY_LABELS', () => {
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
