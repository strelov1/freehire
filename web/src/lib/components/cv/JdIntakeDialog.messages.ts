import { defineMessages } from '$lib/i18n/t';

export const messages = defineMessages(
  {
    dialogTitle: 'Tailor a CV for a job',
    tabs: {
      existing: 'Our vacancy',
      url: 'Link',
      text: 'Paste text',
    },
    searchLabel: 'Search our catalog',
    searchPlaceholder: 'Job title or company…',
    searching: 'Searching…',
    noMatches: 'No matches.',
    unknownCompany: 'Unknown company',
    urlLabel: 'Job posting URL',
    textLabel: 'Job description',
    textPlaceholder: 'Paste the job description here…',
    titleLabel: 'Title',
    optional: '(optional)',
    companyLabel: 'Company',
    resolving: 'Resolving…',
    continue: 'Continue',
    errors: {
      unreadableLink:
        "We couldn't read a vacancy from that link — double-check it, or paste the description as text instead.",
      signInFirst: 'Please sign in first.',
      generic: 'Something went wrong. Please try again.',
    },
  },
  {
    ru: {
      dialogTitle: 'Адаптировать резюме под вакансию',
      tabs: {
        existing: 'Наша вакансия',
        url: 'Ссылка',
        text: 'Вставить текст',
      },
      searchLabel: 'Поиск по нашему каталогу',
      searchPlaceholder: 'Должность или компания…',
      searching: 'Ищем…',
      noMatches: 'Совпадений нет.',
      unknownCompany: 'Неизвестная компания',
      urlLabel: 'Ссылка на вакансию',
      textLabel: 'Описание вакансии',
      textPlaceholder: 'Вставьте описание вакансии сюда…',
      titleLabel: 'Должность',
      optional: '(необязательно)',
      companyLabel: 'Компания',
      resolving: 'Обрабатываем…',
      continue: 'Продолжить',
      errors: {
        unreadableLink:
          'Не удалось прочитать вакансию по этой ссылке — проверьте её или вставьте описание как текст.',
        signInFirst: 'Сначала войдите в аккаунт.',
        generic: 'Что-то пошло не так. Попробуйте ещё раз.',
      },
    },
  },
);
