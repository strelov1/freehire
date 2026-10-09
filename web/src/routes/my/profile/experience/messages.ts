import { defineMessages } from '$lib/i18n/t';

// `ExperienceBankView` itself (the bank editor and its assistant panel) is out of
// scope here — a 700-line shared surface that deserves its own pass, the way
// `JobRow` was deferred in `i18n-my-account-fanout`. This covers only the one
// string this page composes itself: the reseed-failure fallback.
export const messages = defineMessages(
  {
    reseedFailed: 'Could not update your base CV. Try Reset from résumé in a tailoring workspace.',
  },
  {
    ru: {
      reseedFailed: 'Не удалось обновить базовое резюме. Попробуйте «Сбросить из резюме» в мастерской адаптации.',
    },
  },
);
