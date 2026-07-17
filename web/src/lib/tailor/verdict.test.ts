import { describe, it, expect } from 'vitest';
import { splitRequirements } from './verdict';
import type { Analysis } from '$lib/generated/contracts';

function analysis(reqs: Analysis['requirement_match']): Analysis {
  return {
    dimensions: [],
    requirement_match: reqs,
    overall_score: 0,
    verdict: '',
    strengths: [],
    gaps: [],
    recommendation: '',
  };
}

describe('splitRequirements', () => {
  it('splits requirement_match into missing-have and missing-gap, dropping the rest', () => {
    const a = analysis([
      { text: 'Go', priority: 'required', status: 'missing-have', evidence: 'in profile' },
      { text: 'Kubernetes', priority: 'required', status: 'missing-gap', evidence: 'absent' },
      { text: 'REST', priority: 'preferred', status: 'covered', evidence: 'bullet 1' },
      { text: 'SQL', priority: 'preferred', status: 'synonym-only', evidence: 'Postgres' },
    ]);
    const { missingHave, missingGap } = splitRequirements(a);
    expect(missingHave.map((r) => r.text)).toEqual(['Go']);
    expect(missingGap.map((r) => r.text)).toEqual(['Kubernetes']);
  });

  it('returns empty lists for a null analysis or empty requirements', () => {
    expect(splitRequirements(null)).toEqual({ missingHave: [], missingGap: [] });
    expect(splitRequirements(analysis([]))).toEqual({ missingHave: [], missingGap: [] });
  });
});
