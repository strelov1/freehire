import { defineMessages, plurals } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    found: plurals({ one: '{count} job found', other: '{count} jobs found' }),
    show: plurals({ one: 'Show {count} job', other: 'Show {count} jobs' }),
    createAccount: 'Create free account',
    chat: 'Chat',
    backToResults: 'Back to results',
    jobsSearched: 'jobs searched',
    jobsFound: 'jobs found',
    companiesHiring: 'companies hiring',
    unavailable: 'Not available',
    whereJobsAre: 'Where the jobs are',
    companiesSection: 'Companies hiring',
  },
  {
    ru: {
      found: plurals({
        one: 'Найдена {count} вакансия',
        few: 'Найдено {count} вакансии',
        many: 'Найдено {count} вакансий',
        other: 'Найдено {count} вакансии',
      }),
      show: plurals({
        one: 'Показать {count} вакансию',
        few: 'Показать {count} вакансии',
        many: 'Показать {count} вакансий',
        other: 'Показать {count} вакансии',
      }),
      createAccount: 'Создать бесплатный аккаунт',
      chat: 'Чат',
      backToResults: 'К результатам',
      jobsSearched: 'вакансий в поиске',
      jobsFound: 'вакансий найдено',
      companiesHiring: 'компаний нанимают',
      unavailable: 'Нет данных',
      whereJobsAre: 'Где вакансии',
      companiesSection: 'Кто нанимает',
    },
  },
);
