import { describe, it, expect } from 'vitest';
import { workModeLabel, regionLabel, categoryLabel, countryLabel } from './labels';

describe('workModeLabel', () => {
  it('overrides onsite', () => {
    expect(workModeLabel('onsite')).toBe('On-site');
  });

  it('sentence-cases everything else', () => {
    expect(workModeLabel('remote')).toBe('Remote');
    expect(workModeLabel('hybrid')).toBe('Hybrid');
  });
});

describe('regionLabel', () => {
  it('maps known region codes', () => {
    expect(regionLabel('latam')).toBe('LATAM');
    expect(regionLabel('eu')).toBe('Europe');
    expect(regionLabel('global')).toBe('Worldwide');
  });

  it('falls back to sentence case for an unknown region', () => {
    expect(regionLabel('antarctica')).toBe('Antarctica');
  });
});

describe('categoryLabel', () => {
  it('maps a multi-word category', () => {
    expect(categoryLabel('fullstack')).toBe('Full-Stack');
    expect(categoryLabel('ml_ai')).toBe('ML / AI');
  });

  it('falls back to sentence case for an unknown category', () => {
    expect(categoryLabel('quantum_computing')).toBe('Quantum computing');
  });
});

describe('countryLabel', () => {
  it('resolves an ISO code via Intl', () => {
    expect(countryLabel('br')).toBe('Brazil');
    expect(countryLabel('DE')).toBe('Germany');
  });

  it('falls back to the upper-cased code when Intl throws on a malformed one', () => {
    expect(countryLabel('123')).toBe('123');
  });
});
