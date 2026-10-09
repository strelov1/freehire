import { defineMessages } from '$lib/i18n/t';

// `ATSReportView` (the report body itself, shared with the public `/roast` page) is
// out of scope here — deferred the same way `JobRow` was in `i18n-my-account-fanout`.
// This covers only the page's own chrome: the review action, load states, and the
// no-CV prompt.
export const messages = defineMessages(
  {
    headTitle: 'CV readiness — freehire',
    runReview: 'Run AI review',
    reRunReview: 'Re-run AI review',
    reviewing: 'Reviewing…',
    reviewUnavailable: 'AI review is not available right now.',
    loadError: "Couldn't load the report.",
    noCv: {
      title: 'Add your CV to score its ATS readiness',
      bodyLead: 'Upload your CV in',
      link: 'profile settings',
      bodyTail: "to check ATS readability and this role's keywords.",
    },
    filterDescription: "Compare your CV's keyword strength against a role, region or seniority you choose.",
  },
  {
    ru: {
      headTitle: 'Готовность резюме для ATS — freehire',
      runReview: 'Запустить AI-проверку',
      reRunReview: 'Запустить AI-проверку повторно',
      reviewing: 'Проверяем…',
      reviewUnavailable: 'AI-проверка сейчас недоступна.',
      loadError: 'Не удалось загрузить отчёт.',
      noCv: {
        title: 'Добавьте резюме, чтобы оценить его готовность для ATS',
        bodyLead: 'Загрузите резюме в',
        link: 'настройках профиля',
        bodyTail: 'чтобы проверить читаемость для ATS и совпадение с ключевыми словами этой роли.',
      },
      filterDescription: 'Сравните, насколько ключевые слова вашего резюме совпадают с ролью, регионом или уровнем, которые вы выберете.',
    },
  },
);
