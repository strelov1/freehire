import { describe, expect, it } from 'vitest';
import { skillDescription, loadSkillDescriptions } from './skillDescriptions';

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
