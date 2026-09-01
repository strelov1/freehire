import { describe, expect, it } from 'vitest';
import { SKILL_DESCRIBED } from './generated/contracts';
import { skillDescription, loadSkillDescriptions, hasSkillDescription } from './skillDescriptions';

// Synchronous on purpose, and the only part of the glossary that is. A chip decides
// whether to draw its "what is this?" affordance while it renders; awaiting a chunk
// first would either pop the affordance in late or draw it on a skill that turns out to
// have no definition — which is the one thing the spec forbids.
describe('hasSkillDescription', () => {
  it('is true for a described skill and false otherwise', () => {
    expect(hasSkillDescription('dbt')).toBe(true);
    expect(hasSkillDescription('some-new-thing')).toBe(false);
    expect(hasSkillDescription('')).toBe(false);
  });

  // Both directions. The dangerous one is the reverse: a slug promising a definition
  // the catalog does not hold draws a "?" that opens an empty reveal. The two are
  // generated in one run so they cannot drift today — this is what would notice if the
  // generator ever stopped emitting them together.
  it('agrees with the catalog in both directions', async () => {
    const catalog = await loadSkillDescriptions();
    for (const slug of Object.keys(catalog)) {
      expect(hasSkillDescription(slug)).toBe(true);
    }
    for (const slug of SKILL_DESCRIBED) {
      expect(catalog[slug]).toBeTruthy();
    }
  });
});

describe('skillDescription', () => {
  it('resolves a described skill', async () => {
    expect(await skillDescription('dbt')).toContain('SQL');
  });

  // A skill no wave has described and a slug that is not a skill answer alike: empty.
  // Every surface renders a described skill differently from an undescribed one, so the
  // absence has to be a value the caller can test rather than a placeholder.
  it('is empty for a skill with no description', async () => {
    expect(await skillDescription('some-new-thing')).toBe('');
    expect(await skillDescription('')).toBe('');
  });
});

describe('loadSkillDescriptions', () => {
  // The catalog is a separate chunk precisely so it is fetched at most once. Handing
  // back the same object proves the memo, and the memo is what keeps a list of chips
  // from asking for the module once per chip.
  it('fetches the catalog once and reuses it', async () => {
    const [first, second] = await Promise.all([loadSkillDescriptions(), loadSkillDescriptions()]);
    expect(first).toBe(second);
    expect(await loadSkillDescriptions()).toBe(first);
  });

  it('exposes every described skill', async () => {
    const catalog = await loadSkillDescriptions();
    expect(Object.keys(catalog).length).toBeGreaterThan(0);
    for (const [slug, description] of Object.entries(catalog)) {
      expect(slug).toBe(slug.toLowerCase());
      expect(description.trim()).toBe(description);
      expect(description).not.toBe('');
    }
  });
});
