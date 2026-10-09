import { defineMessages } from '$lib/i18n/t';

// `empty.tailorMyCv` names the button on the job page (`JobView.svelte`'s "Tailor my
// CV" CTA), which is not translated yet — the onboarding steps keep quoting it in
// English in every locale rather than claiming a Russian label that doesn't exist on
// screen. Translate it here once that page's own change lands.
export const messages = defineMessages(
  {
    loadFailed: 'Could not load your CVs.',
    deleteFailed: 'Could not delete this CV.',
    loading: 'Loading…',
    empty: {
      title: 'No tailored CVs yet',
      body: "A tailored CV starts from a vacancy's match analysis. Here's how:",
      step1: 'Open a vacancy you want to apply to.',
      step2Lead: 'Press',
      step2Tail: 'on the job page.',
      step3Lead: 'On the result, choose',
      step3Tail: '— your tailored copy appears here.',
      tailorMyCv: 'Tailor my CV',
      cta: 'Tailor for a job',
    },
    updated: 'Updated {date}',
    openPdf: 'Open PDF',
    deleteTitle: 'Delete',
    openAria: 'Open {title}',
    deleteDialog: {
      title: 'Delete your tailored CV for “{title}”?',
      description: 'This cannot be undone.',
      confirm: 'Delete',
    },
  },
  {
    ru: {
      loadFailed: 'Не удалось загрузить ваши резюме.',
      deleteFailed: 'Не удалось удалить это резюме.',
      loading: 'Загрузка…',
      empty: {
        title: 'Пока нет адаптированных резюме',
        body: 'Адаптированное резюме начинается с анализа соответствия вакансии. Вот как это сделать:',
        step1: 'Откройте вакансию, на которую хотите откликнуться.',
        step2Lead: 'Нажмите',
        step2Tail: 'на странице вакансии.',
        step3Lead: 'В результате выберите',
        step3Tail: '— ваша адаптированная копия появится здесь.',
        tailorMyCv: 'Tailor my CV',
        cta: 'Адаптировать под вакансию',
      },
      updated: 'Обновлено {date}',
      openPdf: 'Открыть PDF',
      deleteTitle: 'Удалить',
      openAria: 'Открыть {title}',
      deleteDialog: {
        title: 'Удалить адаптированное резюме для «{title}»?',
        description: 'Это действие нельзя отменить.',
        confirm: 'Удалить',
      },
    },
  },
);
