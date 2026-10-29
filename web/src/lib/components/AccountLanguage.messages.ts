import { defineMessages } from '#lib/i18n/t.js';
import type { Locale } from '#lib/locale.js';

export const messages = defineMessages(
  {
    heading: 'Language',
    description:
      'The language of these account pages and of the assistant’s replies. CVs and cover letters follow each vacancy’s own language. The account interface is available in English and Russian.',
    saving: 'Saving…',
    saved: 'Saved',
    saveFailed: 'Could not save.',
    searchPlaceholder: 'Search a language…',
    noMatches: 'No matches',
    // Spoken after the saved language's name; the check mark beside it is visual only.
    currentChoice: '(current)',
    // Display names for the picker's entries — a language CODE (`ru`, `es`, …)
    // stays a wire token, but the name a reader sees in the combobox is prose.
    // Typed as `Record<Locale, string>` so adding a locale to `SUPPORTED_LOCALES`
    // without a matching entry here is a compile error, not a silent `tokenLabel`
    // fallback to the raw code in the picker.
    languageNames: {
      en: 'English',
      ru: 'Russian',
      es: 'Spanish',
      pt: 'Portuguese',
      de: 'German',
      fr: 'French',
    } satisfies Record<Locale, string>,
  },
  {
    ru: {
      heading: 'Язык',
      description:
        'Язык страниц аккаунта и ответов ассистента. Резюме и сопроводительные письма пишутся на языке вакансии. Интерфейс аккаунта доступен на английском и русском.',
      saving: 'Сохраняем…',
      saved: 'Сохранено',
      saveFailed: 'Не удалось сохранить.',
      searchPlaceholder: 'Поиск языка…',
      noMatches: 'Совпадений нет',
      currentChoice: '(текущий)',
      languageNames: {
        en: 'Английский',
        ru: 'Русский',
        es: 'Испанский',
        pt: 'Португальский',
        de: 'Немецкий',
        fr: 'Французский',
      },
    },
  },
);
