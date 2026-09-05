import { describe, it, expect } from 'vitest';
import { PLACEHOLDER_ROLES, rolePlaceholders } from './placeholderRoles';
import { CATEGORY_VALUES } from './generated/contracts';
import { categoryLabel } from './labels';

// The jobs search box's rotating examples. The list is category KEYS, not display
// strings, so these tests are mostly about that guarantee holding at runtime too:
// the type check is what catches a retired category, and a runtime assertion is what
// catches the list being reordered into something the box would render as a blank.

describe('PLACEHOLDER_ROLES', () => {
  it('is a non-empty list', () => {
    expect(PLACEHOLDER_ROLES.length).toBeGreaterThan(0);
  });

  it('holds no duplicates — a repeat reads as the rotation having stalled', () => {
    expect(new Set(PLACEHOLDER_ROLES).size).toBe(PLACEHOLDER_ROLES.length);
  });

  it('names only categories the backend vocabulary carries', () => {
    for (const role of PLACEHOLDER_ROLES) {
      expect(CATEGORY_VALUES).toContain(role);
    }
  });

  it('resolves every key to a non-empty label', () => {
    for (const role of PLACEHOLDER_ROLES) {
      expect(categoryLabel(role).trim()).not.toBe('');
    }
  });
});

describe('rolePlaceholders', () => {
  // Pinned literally rather than re-derived from PLACEHOLDER_ROLES + categoryLabel:
  // mapping the same inputs through the same functions would restate the implementation
  // and pass however the wording changed. These are the strings that ship.
  it('composes the placeholders a visitor actually reads', () => {
    expect(rolePlaceholders()).toEqual([
      'Search jobs — e.g. Backend',
      'Search jobs — e.g. Frontend',
      'Search jobs — e.g. DevOps',
      'Search jobs — e.g. QA',
      'Search jobs — e.g. Data Science',
      'Search jobs — e.g. Product',
    ]);
  });

  // Under prefers-reduced-motion the first entry is the only one anyone sees, so it
  // carries the whole weight of answering "what can I type here".
  it('leads with a role rather than the catch-all category', () => {
    expect(PLACEHOLDER_ROLES[0]).not.toBe('other');
  });
});
