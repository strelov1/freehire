import { describe, expect, it } from 'vitest';
import { defineMessages, t } from './t';

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
      title: 'Безопасность',
      password: {
        heading: 'Пароль',
        // `save` intentionally omitted — must fall back to English per key.
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

  it('falls back to English per key when a Russian key is missing', () => {
    expect(t(messages, 'ru').password.save).toBe('Save');
  });

  it('falls back to English for a locale with no dictionary at all', () => {
    expect(t(messages, 'es')).toEqual(t(messages, 'en'));
    expect(t(messages, 'pt')).toEqual(t(messages, 'en'));
  });
});

describe('defineMessages / t with a string-list leaf', () => {
  const messages = defineMessages(
    { items: ['One', 'Two', 'Three'] },
    { items: ['Один', 'Два', 'Три'] },
  );

  it('replaces the whole list for a locale that provides one', () => {
    expect(t(messages, 'ru').items).toEqual(['Один', 'Два', 'Три']);
  });

  it('falls back to the English list when untranslated', () => {
    expect(t(messages, 'es').items).toEqual(['One', 'Two', 'Three']);
  });
});
