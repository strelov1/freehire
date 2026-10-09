import { defineMessages } from '$lib/i18n/t';

// `e.signal` (stage_set/employer_reply) is a raw wire token, not catalog text.
export const messages = defineMessages(
  {
    applied: 'Applied',
    employerRepliedWithSignal: 'Employer replied — {signal}',
    employerReplied: 'Employer replied',
    followedUp: 'Followed up',
    movedTo: 'Moved to {signal}',
    stageChanged: 'Stage changed',
    interviewScheduled: 'Interview scheduled',
  },
  {
    ru: {
      applied: 'Отклик отправлен',
      employerRepliedWithSignal: 'Работодатель ответил — {signal}',
      employerReplied: 'Работодатель ответил',
      followedUp: 'Напомнили о себе',
      movedTo: 'Переход на этап {signal}',
      stageChanged: 'Этап изменён',
      interviewScheduled: 'Собеседование назначено',
    },
  },
);
