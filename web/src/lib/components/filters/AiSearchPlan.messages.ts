import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    intro: "Here's the search I'd run:",
    notRecognised: "Didn't recognise: {values}",
    askRun: 'Want me to run this search, or change anything first?',
    cardTitle: 'Your search plan is ready',
    cardBody: 'Run it now or keep refining it in chat.',
    run: 'Run search',
    keepChatting: 'Keep chatting',
  },
  {
    ru: {
      intro: 'Вот поиск, который я бы запустил:',
      notRecognised: 'Не распознал: {values}',
      askRun: 'Запустить этот поиск или сначала что-то поменять?',
      cardTitle: 'План поиска готов',
      cardBody: 'Запустите его сейчас или уточните в чате.',
      run: 'Запустить поиск',
      keepChatting: 'Продолжить чат',
    },
  },
);
