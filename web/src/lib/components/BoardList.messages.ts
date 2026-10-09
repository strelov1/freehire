import { defineMessages, plurals } from '$lib/i18n/t';

// `groupedStages()`'s options come from the generated pipeline-stage vocabulary —
// stays English, same boundary as JobBoard.messages.ts.
export const messages = defineMessages(
  {
    empty: 'No applications yet.',
    unknownCompany: 'Unknown company',
    noReplyTooltip: plurals({ one: 'No reply for {days} day', other: 'No reply for {days} days' }),
    linkedEmails: plurals({ one: '{count} linked email', other: '{count} linked emails' }),
    stageAria: 'Stage for {title} at {company}',
    unknownCompanyLower: 'unknown company',
    noStage: 'No stage',
  },
  {
    ru: {
      empty: 'Пока нет заявок.',
      unknownCompany: 'Неизвестная компания',
      noReplyTooltip: plurals({
        one: 'Нет ответа {days} день',
        few: 'Нет ответа {days} дня',
        many: 'Нет ответа {days} дней',
        other: 'Нет ответа {days} дня',
      }),
      linkedEmails: plurals({
        one: '{count} привязанное письмо',
        few: '{count} привязанных письма',
        many: '{count} привязанных писем',
        other: '{count} привязанных писем',
      }),
      stageAria: 'Этап для «{title}» в компании {company}',
      unknownCompanyLower: 'неизвестная компания',
      noStage: 'Без этапа',
    },
  },
);
