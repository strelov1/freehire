import { describe, expect, it } from 'vitest';
import { isLocale, LOCALE_COOKIE, SUPPORTED_LOCALES } from './locale';

describe('isLocale', () => {
  it('accepts every supported locale code', () => {
    for (const code of SUPPORTED_LOCALES) {
      expect(isLocale(code)).toBe(true);
    }
  });

  it('rejects an unsupported code', () => {
    expect(isLocale('xx')).toBe(false);
    expect(isLocale('EN')).toBe(false);
  });

  it('rejects null, undefined, and the empty string', () => {
    expect(isLocale(null)).toBe(false);
    expect(isLocale(undefined)).toBe(false);
    expect(isLocale('')).toBe(false);
  });
});

describe('LOCALE_COOKIE', () => {
  it('is a stable, distinct cookie name', () => {
    expect(LOCALE_COOKIE).toBe('hire_lang');
  });
});
