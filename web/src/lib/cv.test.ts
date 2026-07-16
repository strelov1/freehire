import { describe, it, expect } from 'vitest';
import { emptyDocument, toEditable, cvTitle, blankExperience } from './cv';

describe('emptyDocument', () => {
  it('populates every section so the form can bind without null-guards', () => {
    const d = emptyDocument();
    expect(d.header.full_name).toBe('');
    expect(d.experience).toEqual([]);
    expect(d.skills).toEqual([]);
    expect(d.certifications).toEqual([]);
  });
});

describe('toEditable', () => {
  it('fills omitted sections from a sparse API document', () => {
    const d = toEditable({ header: { full_name: 'Ada' } });
    expect(d.header.full_name).toBe('Ada');
    expect(d.header.links).toEqual([]);
    expect(d.summary).toBe('');
    expect(d.experience).toEqual([]);
  });

  it('preserves provided sections', () => {
    const d = toEditable({
      header: { full_name: 'Ada' },
      experience: [{ role: 'Eng', bullets: ['did x'] }],
    });
    expect(d.experience).toHaveLength(1);
    expect(d.experience?.[0]?.role).toBe('Eng');
  });
});

describe('cvTitle', () => {
  it('defaults blank/whitespace titles', () => {
    expect(cvTitle('')).toBe('Untitled CV');
    expect(cvTitle('   ')).toBe('Untitled CV');
    expect(cvTitle('  Backend CV ')).toBe('Backend CV');
  });
});

describe('blankExperience', () => {
  it('starts with one empty bullet so the row shows a bullet input', () => {
    expect(blankExperience().bullets).toEqual(['']);
  });
});
