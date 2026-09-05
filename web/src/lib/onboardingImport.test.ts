import { describe, it, expect } from 'vitest';
import { mergeFacets, type StagedFacets } from './onboardingImport';
import { MAX_SPECIALIZATIONS } from './profileLimits';
import type { ResumeProfile } from './types';

function staged(over: Partial<StagedFacets> = {}): StagedFacets {
  return { specializations: [], seniorities: [], skills: [], ...over };
}

function imported(over: Partial<ResumeProfile> = {}): ResumeProfile {
  return { categories: [], skills: [], ...over };
}

describe('mergeFacets', () => {
  it('adds what the import resolved to what is already staged', () => {
    const got = mergeFacets(
      staged({ skills: ['go'], specializations: ['backend'] }),
      imported({ skills: ['python'], categories: ['data'], seniority: 'senior' }),
    );

    expect(got.skills).toEqual(['go', 'python']);
    expect(got.specializations).toEqual(['backend', 'data']);
    expect(got.seniorities).toEqual(['senior']);
  });

  // The wizard reappears until a CV exists, so the staged set routinely carries a previous
  // visit's picks. Replacing them would silently undo work the user already did.
  it('never drops a staged value the import did not mention', () => {
    const got = mergeFacets(staged({ seniorities: ['staff'], skills: ['go'] }), imported({ skills: ['python'] }));

    expect(got.seniorities).toEqual(['staff']);
    expect(got.skills).toContain('go');
  });

  it('leaves a field untouched when the import resolved nothing for it', () => {
    const before = staged({ specializations: ['backend'], seniorities: ['senior'], skills: ['go'] });
    const got = mergeFacets(before, imported());

    expect(got.specializations).toBe(before.specializations);
    expect(got.seniorities).toBe(before.seniorities);
    expect(got.skills).toBe(before.skills);
  });

  it('does not duplicate a value both sides hold', () => {
    const got = mergeFacets(staged({ skills: ['go', 'python'] }), imported({ skills: ['python', 'rust'] }));
    expect(got.skills).toEqual(['go', 'python', 'rust']);
  });

  it('adds a second level rather than replacing the first', () => {
    const got = mergeFacets(staged({ seniorities: ['mid'] }), imported({ seniority: 'senior' }));
    expect(got.seniorities).toEqual(['mid', 'senior']);
  });

  // The server rejects the whole save past the cap, so an uncapped union here turned a good
  // import into a profile that could not be saved at all — the 400 a candidate hit on prod.
  describe('the specialization cap', () => {
    const many = (n: number) => Array.from({ length: n }, (_, i) => `category-${i}`);

    it('keeps at most the cap and reports what it left out', () => {
      const got = mergeFacets(staged(), imported({ categories: many(MAX_SPECIALIZATIONS + 3) }));

      expect(got.specializations).toHaveLength(MAX_SPECIALIZATIONS);
      expect(got.specializationsDropped).toBe(3);
    });

    it('drops the overflow from the import, never what the user already picked', () => {
      const mine = ['backend', 'data'];
      const got = mergeFacets(staged({ specializations: mine }), imported({ categories: many(MAX_SPECIALIZATIONS) }));

      expect(got.specializations.slice(0, 2)).toEqual(mine);
      expect(got.specializations).toHaveLength(MAX_SPECIALIZATIONS);
      expect(got.specializationsDropped).toBe(2);
    });

    it('reports nothing dropped when the import fits', () => {
      const got = mergeFacets(staged({ specializations: ['backend'] }), imported({ categories: ['data'] }));
      expect(got.specializationsDropped).toBe(0);
    });
  });

  describe('resolved', () => {
    it('is false when the import recognised nothing', () => {
      expect(mergeFacets(staged(), imported()).resolved).toBe(false);
    });

    it.each([
      ['a category', imported({ categories: ['backend'] })],
      ['a skill', imported({ skills: ['go'] })],
      ['a seniority', imported({ seniority: 'senior' })],
    ])('is true when the import recognised %s', (_label, incoming) => {
      expect(mergeFacets(staged(), incoming).resolved).toBe(true);
    });

    // Recognising something is not the same as changing something. A user who already
    // picked "go" and then imports a profile that says "go" was read correctly, and telling
    // them we could not read it would be wrong.
    it('is true even when everything it resolved was already staged', () => {
      const got = mergeFacets(staged({ skills: ['go'] }), imported({ skills: ['go'] }));
      expect(got.resolved).toBe(true);
      expect(got.skills).toEqual(['go']);
    });
  });
});
