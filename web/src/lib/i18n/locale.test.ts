import { describe, expect, it } from 'vitest';
import {
  isLocale,
  isTranslatedLocale,
  LOCALE_COOKIE,
  SUPPORTED_LOCALES,
  TRANSLATED_LOCALES,
} from './locale';

describe('isLocale', () => {
  it('accepts every supported locale', () => {
    for (const locale of SUPPORTED_LOCALES) expect(isLocale(locale)).toBe(true);
  });

  it('rejects an unsupported code', () => {
    expect(isLocale('xx')).toBe(false);
    // Case-sensitive: `users.language` stores the lowercase form, and accepting
    // 'EN' here would admit a value the backend's CHECK constraint would not.
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

describe('isTranslatedLocale', () => {
  it('accepts every translated locale', () => {
    for (const locale of TRANSLATED_LOCALES) expect(isTranslatedLocale(locale)).toBe(true);
  });

  it('rejects a supported locale nobody has translated yet', () => {
    // This is the whole point of the second list. `users.language` accepts `es`,
    // the account picker offers it, and `t()` would resolve an `es` catalog the
    // moment one existed — but until then the resolvers must NOT hand `es` to
    // `<html lang>`, or the attribute names a language the page is not in.
    const untranslated = SUPPORTED_LOCALES.filter((l) => !TRANSLATED_LOCALES.includes(l));
    expect(untranslated).not.toEqual([]);
    for (const locale of untranslated) expect(isTranslatedLocale(locale)).toBe(false);
  });

  it('rejects an unknown, empty, null or undefined value', () => {
    expect(isTranslatedLocale('xx')).toBe(false);
    expect(isTranslatedLocale('')).toBe(false);
    expect(isTranslatedLocale(null)).toBe(false);
    expect(isTranslatedLocale(undefined)).toBe(false);
  });
});

describe('the two locale lists', () => {
  it('keeps every translated locale within the supported set', () => {
    // A locale here that `users.language` does not accept could never be
    // resolved, and would look like a translation that simply never renders.
    const unsupported = TRANSLATED_LOCALES.filter((l) => !isLocale(l));
    expect(unsupported, 'translated locales missing from SUPPORTED_LOCALES').toEqual([]);
  });

  it('always translates English', () => {
    expect(TRANSLATED_LOCALES).toContain('en');
  });
});
