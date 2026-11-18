import { defineMessages } from '#lib/i18n/t.js';

// `empty.tailorMyCv` names the button on the job page (`JobView.svelte`'s "Tailor my
// CV" CTA), which is not translated yet — the onboarding steps keep quoting it in
// English in every locale rather than claiming a Russian label that doesn't exist on
// screen. Translate it here once that page's own change lands.
export const messages = defineMessages(
  {
    heading: 'Tailored CVs',
    loadFailed: 'Could not load your CVs.',
    deleteFailed: 'Could not delete this CV.',
    deleted: 'CV deleted',
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
    base: {
      heading: 'Base CVs',
      description: 'Edit your original CV or create a separate version for a job.',
      loadFailed: 'Could not load your base CVs.',
      defaultBadge: 'Default',
      templateLine: 'Template: {template}',
      edit: 'Edit',
      tailor: 'Tailor CV to a job',
      duplicate: 'Duplicate',
      duplicateFailed: 'Could not duplicate this CV.',
      deleteDialog: {
        title: 'Delete the base CV “{title}”?',
        description: 'CVs already tailored from it keep working. This cannot be undone.',
      },
      empty: {
        title: 'No base CV yet',
        body: 'A base CV is built from your uploaded CV and work history. Create one to edit it and tailor from it.',
        cta: 'Create from my CV',
        failed: 'Could not create a base CV. Upload your CV first.',
      },
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
      heading: 'Адаптированные резюме',
      loadFailed: 'Не удалось загрузить ваши резюме.',
      deleteFailed: 'Не удалось удалить это резюме.',
      deleted: 'Резюме удалено',
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
      base: {
        heading: 'Базовые резюме',
        description: 'Редактируйте исходное резюме или создайте отдельную версию под вакансию.',
        loadFailed: 'Не удалось загрузить базовые резюме.',
        defaultBadge: 'По умолчанию',
        templateLine: 'Шаблон: {template}',
        edit: 'Редактировать',
        tailor: 'Адаптировать резюме',
        duplicate: 'Дублировать',
        duplicateFailed: 'Не удалось дублировать это резюме.',
        deleteDialog: {
          title: 'Удалить базовое резюме «{title}»?',
          description: 'Адаптированные из него копии продолжат работать. Это действие нельзя отменить.',
        },
        empty: {
          title: 'Базового резюме пока нет',
          body: 'Базовое резюме собирается из вашего загруженного резюме и истории работы. Создайте его, чтобы править и адаптировать из него.',
          cta: 'Создать из моего резюме',
          failed: 'Не удалось создать базовое резюме. Сначала загрузите резюме.',
        },
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
