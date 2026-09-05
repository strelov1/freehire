import { defineMessages } from '$lib/i18n/t';

export const messages = defineMessages(
  {
    headTitle: 'Contribute a board — freehire',
    signedOut: 'Sign in to contribute a board.',
    title: 'Contribute a board',
    description:
      "Found a company we don't cover yet? Paste any link from its ATS careers page — a vacancy or the board itself. If it's a board we don't crawl, we add it and pull in all of its jobs.",
    urlLabel: 'Job URL',
    submit: 'Contribute',
    submitting: 'Checking…',
    // A malformed link is the only client-side failure; anything else the intake
    // recognises comes back as an outcome, not an error.
    submitFailed: 'Could not submit the link. Please try again.',
    listHeading: 'My contributions',
    loadError: "Couldn't load your contributions.",
    empty: 'No boards yet. Paste an ATS link to get started.',
    // A review row reads "Under review · not credited yet · 2 days ago · via web";
    // any other row reads "<source> · contributed 2 days ago".
    underReview: 'Under review',
    notCreditedYet: 'not credited yet',
    contributedPrefix: 'contributed',
    // " · via <surface>" — the surface is a wire token (`extension`, `cli`), not
    // display text, so only the connective is translated.
    viaPrefix: 'via',
  },
  {
    ru: {
      headTitle: 'Добавить борд — freehire',
      signedOut: 'Войдите, чтобы добавить борд.',
      title: 'Добавить борд',
      description:
        'Нашли компанию, которой у нас ещё нет? Вставьте любую ссылку с её карьерной страницы — саму вакансию или борд целиком. Если этот борд мы не обходим, добавим его и заберём все его вакансии.',
      urlLabel: 'Ссылка на вакансию',
      submit: 'Добавить',
      submitting: 'Проверяем…',
      submitFailed: 'Не удалось отправить ссылку. Попробуйте ещё раз.',
      listHeading: 'Мои добавления',
      loadError: 'Не удалось загрузить ваши добавления.',
      empty: 'Пока пусто. Вставьте ссылку на ATS, чтобы начать.',
      underReview: 'На проверке',
      notCreditedYet: 'ещё не засчитано',
      contributedPrefix: 'добавлено',
      viaPrefix: 'через',
    },
  },
);
