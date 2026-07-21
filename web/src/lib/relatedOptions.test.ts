import { describe, it, expect } from 'vitest';
import { baseRole, optionMatches, relatedOptions, type FacetOption } from './facets';
import { ROLE_RELATED } from './roleRelated';
import { ROLE_LABELS } from './generated/contracts';

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

describe('ROLE_RELATED integrity', () => {
  // Every hub key and every suggested relative must be a real catalog slug: a typo
  // makes the suggestion silently inert (relatedOptions drops slugs absent from the
  // distribution), so only a test catches it. Keys and values are BASE slugs and the
  // catalog carries the base slug for every gradeable role, so a plain membership
  // check suffices — no need to strip a grade.
  const catalog = ROLE_LABELS as Record<string, string>;

  it('every hub key exists in ROLE_LABELS', () => {
    const missing = Object.keys(ROLE_RELATED).filter((slug) => !(slug in catalog));
    expect(missing).toEqual([]);
  });

  it('every suggested relative exists in ROLE_LABELS', () => {
    const missing = [...new Set(Object.values(ROLE_RELATED).flat())].filter(
      (slug) => !(slug in catalog),
    );
    expect(missing).toEqual([]);
  });

  it('a hub never suggests itself', () => {
    const selfRefs = Object.entries(ROLE_RELATED).filter(([hub, rel]) => rel.includes(hub));
    expect(selfRefs).toEqual([]);
  });
});

describe('optionMatches', () => {
  const aliases = {
    software_engineer: ['swe', 'sde', 'software engineer'],
    sre: ['sre', 'site reliability'],
    senior: ['senior', 'sr'],
  };
  const label = (value: string, l: string): FacetOption => ({ value, label: l });

  it('matches on the label (with no aliases needed)', () => {
    expect(optionMatches(label('sre', 'Site Reliability Engineer'), 'reliability')).toBe(true);
  });

  it('matches a shorthand alias the label would miss', () => {
    // "swe" is nowhere in "Software Engineer" — only the alias bridges it.
    expect(optionMatches(label('software_engineer', 'Software Engineer'), 'swe', aliases)).toBe(true);
    expect(optionMatches(label('sre', 'Site Reliability Engineer'), 'sre', aliases)).toBe(true);
  });

  it('resolves the alias by BASE slug, so a graded variant matches too', () => {
    // senior_software_engineer → base software_engineer → "swe" alias.
    expect(
      optionMatches(label('senior_software_engineer', 'Senior Software Engineer'), 'swe', aliases),
    ).toBe(true);
    // seniority-only role via its own alias.
    expect(optionMatches(label('senior', 'Senior'), 'sr', aliases)).toBe(true);
  });

  it('does not match unrelated queries, and needs the alias map to use aliases', () => {
    expect(optionMatches(label('software_engineer', 'Software Engineer'), 'swe')).toBe(false);
    expect(optionMatches(label('software_engineer', 'Software Engineer'), 'devops', aliases)).toBe(false);
  });

  it('an empty query matches everything', () => {
    expect(optionMatches(label('sre', 'Site Reliability Engineer'), '', aliases)).toBe(true);
  });
});
