import { defineMessages } from '#lib/i18n/t.js';

// `e.signal` on employer_reply is a raw wire token, not catalog text; on stage_set it is a
// stage key, rendered through `humanizeStage` into `{stage}`.
export const messages = defineMessages(
  {
    applied: 'Applied',
    employerRepliedWithSignal: 'Employer replied — {signal}',
    employerReplied: 'Employer replied',
    followedUp: 'Followed up',
    movedTo: 'Moved to {stage}',
    stageChanged: 'Stage changed',
    interviewScheduled: 'Interview scheduled',
  },
  {
    ru: {
      applied: 'Отклик отправлен',
      employerRepliedWithSignal: 'Работодатель ответил — {signal}',
      employerReplied: 'Работодатель ответил',
      followedUp: 'Напомнили о себе',
      movedTo: 'Переход на этап «{stage}»',
      stageChanged: 'Этап изменён',
      interviewScheduled: 'Собеседование назначено',
    },
  },
);
