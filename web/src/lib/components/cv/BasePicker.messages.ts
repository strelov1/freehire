import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    label: 'Which base CV should this start from?',
    defaultBadge: 'Default',
    edited: 'Edited {date}',
    copyHint: 'A job you already tailored a CV for reopens that CV as it is, whichever base you pick.',
  },
  {
    ru: {
      label: 'С какого базового резюме начать?',
      defaultBadge: 'По умолчанию',
      edited: 'Изменено {date}',
      copyHint:
        'Вакансия, под которую вы уже адаптировали резюме, откроется с этой же копией — какую бы базу вы ни выбрали.',
    },
  },
);
