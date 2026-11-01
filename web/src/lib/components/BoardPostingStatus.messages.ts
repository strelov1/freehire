import { defineMessages, plurals } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    closed: 'Vacancy closed',
    applyBy: 'Apply by',
    deadlinePassed: 'Application deadline passed',
    lastDay: 'Last day to apply',
    daysLeft: plurals({ one: '{days} day left', other: '{days} days left' }),
  },
  {
    ru: {
      closed: 'Вакансия закрыта',
      applyBy: 'Подать до',
      deadlinePassed: 'Срок подачи истёк',
      lastDay: 'Последний день подачи',
      daysLeft: plurals({
        one: 'Остался {days} день',
        few: 'Осталось {days} дня',
        many: 'Осталось {days} дней',
        other: 'Осталось {days} дня',
      }),
    },
  },
);
