import { defineMessages, plurals } from '$lib/i18n/t';

// `dayHeading`/`clockOf` pass `undefined` to `toLocaleDateString`/`toLocaleTimeString`
// (the BROWSER's locale, not the account's) — a known, already-documented gap
// (design.md's "Dates and relative times follow the BROWSER's locale"); threading the
// resolved locale through is that change's own scope, not this one's.
export const messages = defineMessages(
  {
    weekdays: ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'],
    previousMonth: 'Previous month',
    nextMonth: 'Next month',
    loadError: "Couldn't load what happened this month.",
    interviewCancelled: 'Interview — cancelled',
    interview: 'Interview',
    entries: plurals({ one: '{count} entry', other: '{count} entries' }),
    interviewScheduledSuffix: ', interview scheduled',
    cancelledSuffix: ' — cancelled',
    unconfirmedSuffix: ' — unconfirmed',
    applicationLink: 'application',
    joinLink: 'join',
    nothingThisDay: 'Nothing happened on this day.',
    connectBanner: {
      lead: 'Connect your calendar and the interviews you accept will appear here, on the day they are due.',
      detail:
        'Only meetings we can attach to one of your applications are stored — the rest of your calendar is read and discarded. While our Google app is awaiting verification the connection works for approved test accounts only.',
      cta: 'Connect in Integrations',
    },
    emptyMonthLead:
      'Nothing recorded in {month}. Applications you track, and the replies they get, appear here — start from the',
    boardLink: 'board',
  },
  {
    ru: {
      weekdays: ['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс'],
      previousMonth: 'Предыдущий месяц',
      nextMonth: 'Следующий месяц',
      loadError: 'Не удалось загрузить события этого месяца.',
      interviewCancelled: 'Собеседование — отменено',
      interview: 'Собеседование',
      entries: plurals({
        one: '{count} запись',
        few: '{count} записи',
        many: '{count} записей',
        other: '{count} записи',
      }),
      interviewScheduledSuffix: ', назначено собеседование',
      cancelledSuffix: ' — отменено',
      unconfirmedSuffix: ' — не подтверждено',
      applicationLink: 'заявка',
      joinLink: 'подключиться',
      nothingThisDay: 'В этот день ничего не было.',
      connectBanner: {
        lead: 'Подключите календарь — собеседования, которые вы приняли, будут появляться здесь в свой день.',
        detail:
          'Сохраняются только встречи, которые можно привязать к одной из ваших заявок — остальной календарь читается и отбрасывается. Пока наше приложение Google не прошло проверку, подключение работает только для одобренных тестовых аккаунтов.',
        cta: 'Подключить в Интеграциях',
      },
      emptyMonthLead:
        'В {month} ничего не записано. Здесь появятся отслеживаемые вами заявки и ответы на них — начните с',
      boardLink: 'доски',
    },
  },
);
