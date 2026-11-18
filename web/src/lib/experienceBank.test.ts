import { describe, expect, it } from 'vitest';
import { findFirstUnconfirmed, sortNeedsAttentionFirst } from './experienceBank';
import type { ExperienceAtom, ExperienceBank, ExperienceEmploymentWithAtoms } from './types';

function atom(id: string, provenance: ExperienceAtom['provenance'] = 'manual'): ExperienceAtom {
  return { id, claim: `claim ${id}`, provenance, needs_context: false, needs_metrics: false };
}

function employment(id: string, atoms: ExperienceAtom[]): ExperienceEmploymentWithAtoms {
  return { id, kind: 'job', company: `company ${id}`, atoms };
}

function bank(employments: ExperienceEmploymentWithAtoms[], unplaced: ExperienceAtom[] = []): ExperienceBank {
  return { employments, unplaced };
}

describe('findFirstUnconfirmed', () => {
  it('finds an unconfirmed atom under the first employment', () => {
    const b = bank([
      employment('e1', [atom('a1'), atom('a2', 'agent_inferred')]),
      employment('e2', [atom('a3', 'agent_inferred')]),
    ]);

    expect(findFirstUnconfirmed(b)).toEqual({ employmentId: 'e1', atomId: 'a2' });
  });

  it('picks the leftmost of two unconfirmed atoms under the same employment', () => {
    const b = bank([employment('e1', [atom('a1'), atom('a2', 'agent_inferred'), atom('a3', 'agent_inferred')])]);

    expect(findFirstUnconfirmed(b)).toEqual({ employmentId: 'e1', atomId: 'a2' });
  });

  it('finds an unconfirmed atom under a later employment when earlier ones have none', () => {
    const b = bank([employment('e1', [atom('a1')]), employment('e2', [atom('a2'), atom('a3', 'agent_inferred')])]);

    expect(findFirstUnconfirmed(b)).toEqual({ employmentId: 'e2', atomId: 'a3' });
  });

  it('finds an unconfirmed atom that has no employment', () => {
    const b = bank([employment('e1', [atom('a1')])], [atom('a2', 'agent_inferred')]);

    expect(findFirstUnconfirmed(b)).toEqual({ employmentId: null, atomId: 'a2' });
  });

  it('returns null when nothing is unconfirmed', () => {
    const b = bank([employment('e1', [atom('a1')])], [atom('a2')]);

    expect(findFirstUnconfirmed(b)).toBeNull();
  });
});

describe('sortNeedsAttentionFirst', () => {
  it('puts unconfirmed atoms before confirmed ones, without mutating the input', () => {
    const original = [atom('a1'), atom('a2', 'agent_inferred'), atom('a3')];

    const sorted = sortNeedsAttentionFirst(original);

    expect(sorted.map((a) => a.id)).toEqual(['a2', 'a1', 'a3']);
    expect(original.map((a) => a.id)).toEqual(['a1', 'a2', 'a3']);
  });

  it('leaves order unchanged when nothing is unconfirmed', () => {
    const sorted = sortNeedsAttentionFirst([atom('a1'), atom('a2')]);

    expect(sorted.map((a) => a.id)).toEqual(['a1', 'a2']);
  });
});
