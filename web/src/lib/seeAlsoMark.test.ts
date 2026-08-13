import { describe, expect, test } from 'vitest';
import { resolveSeeAlsoMark } from './seeAlsoMark';

describe('resolveSeeAlsoMark', () => {
  test('a backer collection gets its brand image, regardless of params', () => {
    const mark = resolveSeeAlsoMark({ skills: 'python' }, '/brands/yc.png');
    expect(mark).toEqual({ kind: 'image', src: '/brands/yc.png' });
  });

  test('a skills param with a curated tech mark gets that brand logo', () => {
    const mark = resolveSeeAlsoMark({ skills: 'python' }, null);
    expect(mark.kind).toBe('logo');
    if (mark.kind === 'logo') {
      expect(mark.hex).toMatch(/^[0-9a-f]{6}$/i);
      expect(mark.path.length).toBeGreaterThan(0);
    }
  });

  test('a skills param with no curated tech mark falls back to the tech family icon', () => {
    const mark = resolveSeeAlsoMark({ skills: 'aws' }, null);
    expect(mark).toEqual({ kind: 'family', icon: 'tech', color: expect.any(String) });
  });

  test('a countries param gets that country flag', () => {
    const mark = resolveSeeAlsoMark({ work_mode: 'remote', countries: 'de' }, null);
    expect(mark).toEqual({ kind: 'flag', countryCode: 'de' });
  });

  test('a seniority param gets the seniority family icon', () => {
    const mark = resolveSeeAlsoMark({ seniority: 'senior' }, null);
    expect(mark).toEqual({ kind: 'family', icon: 'seniority', color: expect.any(String) });
  });

  test('a category param gets the role family icon', () => {
    const mark = resolveSeeAlsoMark({ category: 'backend' }, null);
    expect(mark).toEqual({ kind: 'family', icon: 'role', color: expect.any(String) });
  });

  test('a work_mode param without a concrete country gets the remote family icon', () => {
    const mark = resolveSeeAlsoMark({ work_mode: 'remote', regions: 'global' }, null);
    expect(mark).toEqual({ kind: 'family', icon: 'remote', color: expect.any(String) });
  });

  test('a company-membership collection with no backer image gets the company family icon', () => {
    const mark = resolveSeeAlsoMark({ collections: 'unicorn' }, null);
    expect(mark).toEqual({ kind: 'family', icon: 'company', color: expect.any(String) });
  });

  test('no params and no backer image falls back to the tech family icon', () => {
    const mark = resolveSeeAlsoMark(null, null);
    expect(mark).toEqual({ kind: 'family', icon: 'tech', color: expect.any(String) });
  });
});
