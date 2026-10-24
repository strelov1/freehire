import { defineMessages, plurals } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    jobs: plurals({ one: '{count} job', other: '{count} jobs' }),
    placeCount: '{place}: {jobs}',
    clickToFilter: 'click to filter',
  },
  {
    ru: {
      jobs: plurals({
        one: '{count} вакансия',
        few: '{count} вакансии',
        many: '{count} вакансий',
        other: '{count} вакансии',
      }),
      placeCount: '{place}: {jobs}',
      clickToFilter: 'нажмите, чтобы выбрать',
    },
  },
);
