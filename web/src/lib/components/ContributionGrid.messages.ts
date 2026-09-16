import { defineMessages, plurals } from '$lib/i18n/t';

// The activity view's own catalog. Its vocabulary is deliberately about EFFORT — "actions",
// "active days" — and never about outcomes, because what the grid counts is what the
// candidate did, not what it earned them. A caption that said "events" would invite the
// comparison with Calendar's totals and lose it.
//
// The plural leaves are bare nouns, not "{count} day": the count is placed beside them in the
// markup, the way PlanView's are, so a language that puts the numeral elsewhere is not fought
// with by the catalog.
export const messages = defineMessages(
  {
    headTitle: 'Activity — Tracking — freehire',
    heading: 'Your last year',
    // Said on the page rather than left to be inferred. Someone who sees four events on
    // Calendar and three squares here is owed the reason, and this caption is it.
    caption:
      'One square per day, shaded by what you did — applications, follow-ups, and stages you moved yourself.',
    loadError: "Couldn't load your activity.",
    legendLess: 'Less',
    legendMore: 'More',
    totalLabel: 'Actions this year',
    currentStreakLabel: 'Current streak',
    longestStreakLabel: 'Longest streak',
    days: plurals({ one: 'day', other: 'days' }),
    actions: plurals({ one: 'action', other: 'actions' }),
    noActions: 'No actions',
    // The empty state is an invitation, not a scolding: a blank year is what everybody's
    // first week looks like.
    empty:
      'Nothing here yet. Apply to a job, follow one up, or move an application along, and the day lights up.',
    emptyCta: 'Go to your board',
    panelNothing: 'Nothing happened on this day.',
    recordedByYou: 'recorded by you',
    applicationLink: 'application',
    messageLink: 'message',
  },
  {
    ru: {
      headTitle: 'Активность — Отслеживание — freehire',
      heading: 'Ваш последний год',
      caption:
        'Один квадрат — один день, оттенок зависит от того, что вы сделали: отклики, напоминания и стадии, которые вы передвинули сами.',
      loadError: 'Не удалось загрузить вашу активность.',
      legendLess: 'Меньше',
      legendMore: 'Больше',
      totalLabel: 'Действий за год',
      currentStreakLabel: 'Текущая серия',
      longestStreakLabel: 'Лучшая серия',
      days: plurals({ one: 'день', few: 'дня', many: 'дней', other: 'дня' }),
      actions: plurals({ one: 'действие', few: 'действия', many: 'действий', other: 'действия' }),
      noActions: 'Ничего не было',
      empty:
        'Пока пусто. Откликнитесь на вакансию, напомните о себе или передвиньте отклик — и день загорится.',
      emptyCta: 'Перейти на доску',
      panelNothing: 'В этот день ничего не происходило.',
      recordedByYou: 'записано вами',
      applicationLink: 'отклик',
      messageLink: 'письмо',
    },
  },
);
