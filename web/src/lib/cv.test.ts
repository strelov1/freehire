import { describe, it, expect } from 'vitest';
import {
  emptyDocument,
  toEditable,
  cvTitle,
  blankExperience,
  dateRange,
  experienceHeader,
  educationLine,
  languageLabel,
  certificationLine,
} from './cv';

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

  it('defaults page margins when the document omits them', () => {
    const d = toEditable({ header: { full_name: 'Ada' } });
    expect(d.margins).toEqual({ top: 0.5, right: 0.5, bottom: 0.5, left: 0.5 });
  });

  it('preserves provided page margins', () => {
    const d = toEditable({ header: {}, margins: { top: 0.75, right: 0.4, bottom: 0.75, left: 0.4 } });
    expect(d.margins).toEqual({ top: 0.75, right: 0.4, bottom: 0.75, left: 0.4 });
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

// The preview projections mirror the classic-ats Typst composition rules so the live HTML
// preview reads the same as the rendered PDF (close, not pixel-identical).

describe('dateRange', () => {
  it('joins both ends with an en dash', () => {
    expect(dateRange('2021', '2024')).toBe('2021 – 2024');
  });
  it('shows a single end when the other is blank', () => {
    expect(dateRange('2021', '')).toBe('2021');
    expect(dateRange('', 'Present')).toBe('Present');
  });
  it('is empty when both are blank', () => {
    expect(dateRange('', '')).toBe('');
  });
});

describe('experienceHeader', () => {
  it('joins company | location | role with a trailing date range', () => {
    expect(
      experienceHeader({ company: 'Acme', location: 'Remote', role: 'Eng', start: '2021', end: '2024' }),
    ).toBe('Acme | Remote | Eng (2021 – 2024)');
  });
  it('drops blank parts and omits the parens when there are no dates', () => {
    expect(experienceHeader({ company: 'Acme', role: 'Eng' })).toBe('Acme | Eng');
  });
});

describe('educationLine', () => {
  it('combines degree, field, institution and dates', () => {
    expect(
      educationLine({ degree: 'BSc', field: 'CS', institution: 'MIT', start: '2016', end: '2020' }),
    ).toBe('BSc, CS | MIT (2016 – 2020)');
  });
  it('keeps only the present parts', () => {
    expect(educationLine({ institution: 'MIT' })).toBe('MIT');
  });
});

describe('languageLabel', () => {
  it('appends the level in parens', () => {
    expect(languageLabel({ name: 'English', level: 'C1' })).toBe('English (C1)');
  });
  it('is just the name without a level', () => {
    expect(languageLabel({ name: 'English' })).toBe('English');
  });
});

describe('certificationLine', () => {
  it('joins name — issuer (year)', () => {
    expect(certificationLine({ name: 'CKA', issuer: 'CNCF', year: '2023' })).toBe('CKA — CNCF (2023)');
  });
  it('drops the missing pieces', () => {
    expect(certificationLine({ name: 'CKA' })).toBe('CKA');
  });
});
