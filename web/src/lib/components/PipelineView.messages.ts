import { defineMessages, plurals } from '$lib/i18n/t';

export const messages = defineMessages(
  {
    loadError: "Couldn't load your pipeline.",
    empty: "You haven't applied to any jobs yet. Applications you track will show up here.",
    interviewRate: 'Interview Rate',
    reachedInterview: 'reached interview',
    offerRate: 'Offer Rate',
    reachedOffer: 'reached offer',
    applications: plurals({ one: '{count} application', other: '{count} applications' }),
    yourReplyRate: 'Your Reply Rate',
    withMailbox: '{count} with a connected mailbox',
    averageReplyRate: 'Average Reply Rate',
    everyOtherCandidate: 'every other candidate with a connected mailbox',
    footnote:
      'A snapshot of where your applications stand now. Rates are a lower bound — a job rejected after an interview counts only as rejected.',
  },
  {
    ru: {
      loadError: 'Не удалось загрузить вашу воронку.',
      empty: 'Вы пока не откликались ни на одну вакансию. Здесь появятся отслеживаемые заявки.',
      interviewRate: 'Доля собеседований',
      reachedInterview: 'дошли до собеседования',
      offerRate: 'Доля офферов',
      reachedOffer: 'дошли до оффера',
      applications: plurals({
        one: '{count} заявка',
        few: '{count} заявки',
        many: '{count} заявок',
        other: '{count} заявки',
      }),
      yourReplyRate: 'Ваша доля ответов',
      withMailbox: '{count} с подключённым почтовым ящиком',
      averageReplyRate: 'Средняя доля ответов',
      everyOtherCandidate: 'у всех остальных кандидатов с подключённым почтовым ящиком',
      footnote:
        'Срез того, как обстоят дела с вашими заявками сейчас. Доли — это нижняя граница: вакансия, по которой пришёл отказ после собеседования, считается только как отказ.',
    },
  },
);
