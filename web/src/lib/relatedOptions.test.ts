import { describe, it, expect } from 'vitest';
import { baseRole, relatedOptions, type FacetOption } from './facets';

const opt = (value: string, count = 1): FacetOption => ({ value, label: value, count });

describe('baseRole', () => {
  it('strips a leading seniority grade', () => {
    expect(baseRole('senior_mobile')).toBe('mobile');
    expect(baseRole('lead_software_engineer')).toBe('software_engineer');
    expect(baseRole('c_level_backend')).toBe('backend');
  });
  it('leaves an ungraded slug untouched', () => {
    expect(baseRole('mobile')).toBe('mobile');
    expect(baseRole('ios_developer')).toBe('ios_developer');
    // c_level as a seniority-only role is not a graded prefix of itself.
    expect(baseRole('c_level')).toBe('c_level');
  });
});

describe('relatedOptions', () => {
  const related = {
    mobile: ['ios_developer', 'android_developer', 'react_native_developer'],
    ios_developer: ['android_developer', 'mobile'],
  };
  // A full distribution the picker would have (values that actually have jobs).
  const options = [
    opt('mobile', 100),
    opt('ios_developer', 40),
    opt('android_developer', 30),
    opt('react_native_developer', 10),
  ];

  it('suggests a hub role\'s relatives that the text search did not surface', () => {
    // Typing "mobile" surfaces only "mobile"; its relatives are the payload.
    const out = relatedOptions(options, ['mobile'], [], related);
    expect(out.map((o) => o.value)).toEqual([
      'ios_developer',
      'android_developer',
      'react_native_developer',
    ]);
  });

  it('normalises a graded hub value to its base before lookup', () => {
    const out = relatedOptions(options, ['senior_mobile'], [], related);
    expect(out.map((o) => o.value)).toContain('ios_developer');
  });

  it('excludes relatives already shown (matched) or selected', () => {
    // "ios_developer" is both matched and, say, selected — never re-suggested.
    const out = relatedOptions(options, ['mobile', 'ios_developer'], ['android_developer'], related);
    expect(out.map((o) => o.value)).toEqual(['react_native_developer']);
  });

  it('drops relatives absent from the distribution (no jobs → no label/count)', () => {
    const sparse = [opt('mobile', 5), opt('ios_developer', 2)];
    const out = relatedOptions(sparse, ['mobile'], [], related);
    expect(out.map((o) => o.value)).toEqual(['ios_developer']);
  });

  it('dedupes when several matched hubs point at the same relative', () => {
    const out = relatedOptions(options, ['mobile', 'ios_developer'], [], related);
    // android_developer is a relative of both mobile and ios_developer — once only.
    expect(out.filter((o) => o.value === 'android_developer')).toHaveLength(1);
  });

  it('returns nothing when no matched value is a hub', () => {
    expect(relatedOptions(options, ['android_developer'], [], {})).toEqual([]);
  });

  it('honours the limit', () => {
    const out = relatedOptions(options, ['mobile'], [], related, 2);
    expect(out).toHaveLength(2);
  });
});
