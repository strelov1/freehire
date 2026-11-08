import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    title: 'CVs',
    description: 'Your base CVs, the CVs you tailored for specific roles, and the appearance a new one starts with.',
    tailorForJob: 'Tailor to a job',
    sections: {
      list: 'List',
      template: 'Template',
      typography: 'Typography',
    },
    tabStripLabel: 'CV sections',
  },
  {
    ru: {
      title: 'Резюме',
      description:
        'Ваши базовые резюме, резюме, адаптированные под конкретные роли, и оформление, с которого начинается новое.',
      tailorForJob: 'Адаптировать под вакансию',
      sections: {
        list: 'Список',
        template: 'Шаблон',
        typography: 'Типографика',
      },
      tabStripLabel: 'Разделы резюме',
    },
  },
);
