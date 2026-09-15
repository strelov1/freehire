import { defineMessages } from '$lib/i18n/t';

// One catalogue for the confirmation block itself. The sentence explaining WHY a
// particular action needs confirming stays with that action — an API key and a deleted
// account are not dangerous for the same reason — and is passed in as `prompt`.
export const messages = defineMessages(
  {
    heading: 'Confirm it is you',
    passwordLabel: 'Password',
    providersLoading: 'Loading your sign-in providers…',
    providersError: 'Could not load your sign-in providers.',
    retry: 'Try again',
    // Split around the provider name, which is a brand and never translated.
    confirmWithPrefix: 'Confirm with',
    returnNote: 'We will bring you back here.',
    // Rendered as "Identity confirmed · 9:41". The remaining time is deliberately a bare
    // clock with no word beside it: English puts "left" after the number and Russian puts
    // "осталось" before it, and a prefix/suffix pair to straddle that would be two empty
    // strings in one locale each. A bare duration reads correctly in every language.
    confirmed: 'Identity confirmed',
    noMethod: 'This account has no way to confirm. Please contact support.',
    passwordRequired: 'Enter your password to confirm.',
    wrongPassword: 'That password is not right.',
    // Shown when the server rejects a proof this component believed was held.
    refused: 'Please confirm it is you again.',
  },
  {
    ru: {
      heading: 'Подтвердите, что это вы',
      passwordLabel: 'Пароль',
      providersLoading: 'Загружаем ваши способы входа…',
      providersError: 'Не удалось загрузить ваши способы входа.',
      retry: 'Попробовать ещё раз',
      confirmWithPrefix: 'Подтвердить через',
      returnNote: 'Мы вернём вас сюда.',
      confirmed: 'Личность подтверждена',
      noMethod: 'У этого аккаунта нет способа подтверждения. Напишите в поддержку.',
    },
  },
);
