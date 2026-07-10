import { describe, it, expect } from 'vitest';
import { verdictTone, requirementStatusMeta } from './jobFit';

describe('verdictTone', () => {
  it('buckets scores on the backend thresholds', () => {
    expect(verdictTone(90)).toBe('strong');
    expect(verdictTone(75)).toBe('strong');
    expect(verdictTone(74)).toBe('good');
    expect(verdictTone(60)).toBe('good');
    expect(verdictTone(59)).toBe('moderate');
    expect(verdictTone(45)).toBe('moderate');
    expect(verdictTone(44)).toBe('weak');
    expect(verdictTone(30)).toBe('weak');
    expect(verdictTone(29)).toBe('poor');
    expect(verdictTone(0)).toBe('poor');
  });
});

describe('requirementStatusMeta', () => {
  it('labels and tones each ATS status', () => {
    expect(requirementStatusMeta('covered')).toEqual({ label: 'Covered', tone: 'strong' });
    expect(requirementStatusMeta('synonym-only')).toEqual({ label: 'Synonym', tone: 'good' });
    expect(requirementStatusMeta('missing-have')).toEqual({ label: 'Add it', tone: 'moderate' });
    expect(requirementStatusMeta('missing-gap')).toEqual({ label: 'Gap', tone: 'poor' });
  });

  it('falls back to a neutral label for an unknown status', () => {
    expect(requirementStatusMeta('weird')).toEqual({ label: 'weird', tone: 'moderate' });
  });
});
