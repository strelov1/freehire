import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    open: 'Help me shortlist',
    title: 'Your job shortlist',
    opening: 'Opening your conversation…',
    failed: 'Could not open the shortlist assistant.',
    retry: 'Retry',
    captured: 'Using your captured search',
    viewSearch: 'View search and filters',
    changed: 'Your page filters have changed. This conversation keeps its original search.',
    startCurrent: 'Start from current filters',
    kickoff: 'Help me shortlist up to five vacancies from this search. Explain why each is worth considering and any concerns or unknowns. Use my saved profile if available; ask only about information you need.',
  },
  {
    ru: {
      open: 'Помоги отобрать',
      title: 'Ваша подборка вакансий',
      opening: 'Открываем разговор…',
      failed: 'Не удалось открыть помощника отбора вакансий.',
      retry: 'Повторить',
      captured: 'Используем сохранённый поиск',
      viewSearch: 'Посмотреть поиск и фильтры',
      changed: 'Фильтры на странице изменились. В этом разговоре остаётся исходный поиск.',
      startCurrent: 'Начать с текущих фильтров',
      kickoff: 'Помоги отобрать до пяти вакансий из этого поиска. Объясни, почему их стоит рассмотреть, и отметь сомнения и неизвестные условия. Используй мой сохранённый профиль, если он есть; спрашивай только о необходимых сведениях.',
    },
  },
);
