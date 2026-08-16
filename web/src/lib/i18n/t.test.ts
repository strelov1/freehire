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
