import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    filled: 'Complete',
    empty: 'Not filled',
    optionalFilled: 'Filled',
    optionalEmpty: 'Optional',
    sections: {
      contacts: 'Contacts',
      location: 'Location',
      skills: 'Skills',
      experience: 'Experience',
      education: 'Education',
      screening: 'Screening answers',
      summary: 'Summary',
    },
  },
  {
    ru: {
      filled: 'Заполнено',
      empty: 'Не заполнено',
      optionalFilled: 'Заполнено',
      optionalEmpty: 'Необязательно',
      sections: {
        contacts: 'Контакты',
        location: 'Локация',
        skills: 'Навыки',
        experience: 'Опыт',
        education: 'Образование',
        screening: 'Ответы на вопросы',
        summary: 'Сводка',
      },
    },
  },
);
