import { describe, expect, it } from 'vitest';
import { defineMessages, plural, plurals, t } from './t';

describe('defineMessages / t', () => {
  const messages = defineMessages(
    {
      title: 'Security',
      password: {
        heading: 'Password',
        save: 'Save',
      },
    },
    {
      ru: {
        title: 'Безопасность',
        password: {
          heading: 'Пароль',
          // `save` intentionally omitted — must fall back to English per key.
        },
      },
      // `es` carries a partial translation to prove a third locale needs nothing
      // from this module; `pt`/`de`/`fr` carry none at all.
      es: {
        title: 'Seguridad',
      },
    },
  );

  it('returns the English source for locale "en"', () => {
    expect(t(messages, 'en')).toEqual({
      title: 'Security',
      password: { heading: 'Password', save: 'Save' },
    });
  });

  it('returns the Russian translation for locale "ru"', () => {
    expect(t(messages, 'ru').title).toBe('Безопасность');
    expect(t(messages, 'ru').password.heading).toBe('Пароль');
  });

  it('resolves a third locale from the same catalog', () => {
    expect(t(messages, 'es').title).toBe('Seguridad');
  });

  it('falls back to English per key when a translated key is missing', () => {
    expect(t(messages, 'ru').password.save).toBe('Save');
    expect(t(messages, 'es').password.heading).toBe('Password');
  });

  it('falls back to English for a locale with no translation at all', () => {
    expect(t(messages, 'pt')).toEqual(t(messages, 'en'));
    expect(t(messages, 'de')).toEqual(t(messages, 'en'));
    expect(t(messages, 'fr')).toEqual(t(messages, 'en'));
  });

  it('builds no copy for a locale it was given no translation for', () => {
    // The fallback is a lookup miss, not a pre-built English clone — otherwise
    // every catalog would carry four identical copies for the untranslated
    // locales.
    expect(Object.keys(messages).sort()).toEqual(['en', 'es', 'ru']);
  });
});

describe('plural', () => {
  const en = plurals({ one: 'model call', other: 'model calls' });
  const ru = plurals({
    one: 'обращение',
    few: 'обращения',
    many: 'обращений',
    other: 'обращения',
  });

  it('picks the English singular only for exactly one', () => {
    expect(plural('en', 1, en)).toBe('model call');
    for (const n of [0, 2, 5, 11, 21, 100]) expect(plural('en', n, en)).toBe('model calls');
  });

  it('picks all three Russian forms by the language’s own rule', () => {
    // The rule is not "1 vs many": 21 takes the singular form and 11 does not,
    // which is exactly what a hand-rolled `n === 1` check gets wrong.
    expect(plural('ru', 1, ru)).toBe('обращение');
    expect(plural('ru', 21, ru)).toBe('обращение');
    expect(plural('ru', 2, ru)).toBe('обращения');
    expect(plural('ru', 24, ru)).toBe('обращения');
    expect(plural('ru', 5, ru)).toBe('обращений');
    expect(plural('ru', 11, ru)).toBe('обращений');
    expect(plural('ru', 0, ru)).toBe('обращений');
  });

  it('falls back to `other` for a form the catalog omits', () => {
    // A Russian noun given only English's two forms still renders — as the
    // wrong-but-visible word, never as a blank.
    expect(plural('ru', 5, en)).toBe('model calls');
  });

  it('lets a translation carry categories the English source has no word for', () => {
    // The whole reason plural forms are a leaf. Held to the English shape the way
    // every other key is, Russian could only ever supply `one` and `other` — and
    // "5 обращения" is wrong in a way no fallback can rescue.
    const catalog = defineMessages({ calls: en }, { ru: { calls: ru } });
    expect(plural('ru', 5, t(catalog, 'ru').calls)).toBe('обращений');
    expect(plural('ru', 2, t(catalog, 'ru').calls)).toBe('обращения');
    expect(plural('en', 5, t(catalog, 'en').calls)).toBe('model calls');
  });

  it('replaces the whole form set rather than merging it', () => {
    // Merging would leave English's `other` under a Russian noun, so a count of
    // 2 would read "2 model calls" in an otherwise Russian sentence.
    const catalog = defineMessages({ calls: en }, { ru: { calls: ru } });
    expect(plural('ru', 2, t(catalog, 'ru').calls)).not.toBe('model calls');
  });
});

describe('defineMessages / t with a string-list leaf', () => {
  const messages = defineMessages({ items: ['One', 'Two', 'Three'] }, { ru: { items: ['Один', 'Два', 'Три'] } });

  it('replaces the whole list for a locale that provides one', () => {
    expect(t(messages, 'ru').items).toEqual(['Один', 'Два', 'Три']);
  });

  it('falls back to the English list when untranslated', () => {
    expect(t(messages, 'es').items).toEqual(['One', 'Two', 'Three']);
  });
});
