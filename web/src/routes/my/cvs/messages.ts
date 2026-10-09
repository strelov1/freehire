import { defineMessages } from '$lib/i18n/t';

export const messages = defineMessages(
  {
    title: 'Tailored CVs',
    description: 'CVs you tailored for specific roles, and the appearance a new one starts with.',
    tailorForJob: 'Tailor for a job',
    sections: {
      list: 'List',
      template: 'Template',
      typography: 'Typography',
    },
    tabStripLabel: 'CV sections',
  },
  {
    ru: {
      title: 'Адаптированные резюме',
      description:
        'Резюме, которые вы адаптировали под конкретные роли, и оформление, с которого начинается новое.',
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
