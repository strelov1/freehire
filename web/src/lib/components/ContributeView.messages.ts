import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    headTitle: 'Contribute a board — freehire',
    signedOut: 'Sign in to contribute a board.',
    title: 'Contribute a board',
    description:
      "Paste a LinkedIn job link or an employer's ATS link. We check the vacancy and look for a careers board we can follow.",
    urlLabel: 'Job URL',
    submit: 'Contribute',
    submitting: 'Checking…',
    // A malformed link is the only client-side failure; anything else the intake
    // recognises comes back as an outcome, not an error.
    submitFailed: 'Could not submit the link. Please try again.',
    listHeading: 'My contributions',
    loadError: "Couldn't load your contributions.",
    empty: 'No boards yet. Paste an ATS or LinkedIn job link to get started.',
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
        'Вставьте ссылку на вакансию в LinkedIn или на карьерный сайт работодателя. Проверим вакансию и найдём борд, который сможем отслеживать.',
      urlLabel: 'Ссылка на вакансию',
      submit: 'Добавить',
      submitting: 'Проверяем…',
      submitFailed: 'Не удалось отправить ссылку. Попробуйте ещё раз.',
      listHeading: 'Мои добавления',
      loadError: 'Не удалось загрузить ваши добавления.',
      empty: 'Пока пусто. Вставьте ссылку на ATS или вакансию в LinkedIn, чтобы начать.',
      underReview: 'На проверке',
      notCreditedYet: 'ещё не засчитано',
      contributedPrefix: 'добавлено',
      viaPrefix: 'через',
    },
  },
);
