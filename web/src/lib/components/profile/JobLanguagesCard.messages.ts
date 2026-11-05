import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    heading: 'Job languages',
    description: 'Show vacancies written in any of the selected languages. Leave empty for any language.',
    search: 'Search languages',
    loadFailed: 'Could not load languages. Try again.',
    retry: 'Retry',
  },
  {
    ru: {
      heading: 'Языки вакансий',
      description: 'Показывать вакансии на любом из выбранных языков. Пустой выбор — любой язык.',
      search: 'Поиск языков',
      loadFailed: 'Не удалось загрузить языки. Попробуйте ещё раз.',
      retry: 'Повторить',
    },
  },
);
