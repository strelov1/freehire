import { defineMessages } from '$lib/i18n/t';

export const messages = defineMessages(
  {
    trigger: 'Link to application',
    closeAria: 'Close application picker',
    loadingApps: 'Loading your applications…',
    empty: 'No applications yet — track a job first.',
    searchPlaceholder: 'Search applications…',
    searchAria: 'Search applications',
    unknownCompany: 'Unknown company',
    noMatch: 'No match.',
  },
  {
    ru: {
      trigger: 'Привязать к отклику',
      closeAria: 'Закрыть выбор отклика',
      loadingApps: 'Загружаем ваши отклики…',
      empty: 'Откликов пока нет — сначала отследите вакансию.',
      searchPlaceholder: 'Поиск по откликам…',
      searchAria: 'Поиск откликов',
      unknownCompany: 'Неизвестная компания',
      noMatch: 'Совпадений нет.',
    },
  },
);
