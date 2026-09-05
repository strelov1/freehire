import { defineMessages } from '$lib/i18n/t';

// `ReferralRequestStatus` and an offer's moderation status are wire tokens the view
// used to print raw; both are mapped here, with an unknown value falling through to
// the token rather than to nothing.
export const messages = defineMessages(
  {
    headTitle: 'Referrals — freehire',
    title: 'Referrals',
    tabs: {
      requests: 'My requests',
      offers: 'Offers to refer',
      incoming: 'Incoming',
    },
    requests: {
      empty: "You haven't requested any referrals yet.",
      columns: {
        company: 'Company',
        cvShared: 'CV shared',
        status: 'Status',
        sent: 'Sent',
      },
      cvBuilt: 'Tailored CV',
      cvUploaded: 'Uploaded CV',
      status: {
        sent: 'Sent',
        contacted: 'Contacted',
        declined: 'Declined',
      },
      footnote: 'No notifications here — the referrer contacts you over the channel you left.',
    },
    offers: {
      lead: 'Companies you can refer into.',
      openForm: '+ Offer to refer',
      companyLabel: 'Company',
      companyHint: 'Search and pick the company you work at.',
      linkedinLabel: 'Your LinkedIn profile',
      linkedinHint: 'Helps the moderator confirm you work there.',
      linkedinInvalid: 'Enter a full linkedin.com/in/… profile URL.',
      proofLabel: 'Proof of employment (PDF)',
      proofHint: 'A CV or letter showing you work there. A moderator reviews it.',
      submit: 'Submit for review',
      submitting: 'Submitting…',
      empty: "You haven't offered to refer anywhere yet.",
      status: {
        approved: 'Approved',
        pending: 'Pending review',
        rejected: 'Rejected',
      },
      withdraw: 'Stop referring',
      withdrawing: 'Removing…',
      errors: {
        duplicate: 'You already offered to refer for this company.',
        unknownCompany: "We don't have that company — check the slug in its page URL.",
        badLinkedin: 'Enter a valid LinkedIn profile URL.',
        uploadUnavailable: 'File upload is unavailable right now.',
        generic: 'Could not submit the offer. Please try again.',
      },
      withdrawDialog: {
        // "Stop being a referrer for <company>?" — the company is supplied by the view.
        titlePrefix: 'Stop being a referrer for',
        titleSuffix: '?',
        description: 'You can offer again later.',
        confirmLabel: 'Stop referring',
      },
    },
    incoming: {
      empty: 'No incoming referral requests.',
      // "Someone wants a referral into <company>" — the company is supplied by the view.
      wantsReferralPrefix: 'Someone wants a referral into',
      contactLabel: 'Contact:',
      linkedin: 'LinkedIn ↗',
      viewCv: 'View CV',
      markContacted: 'Mark contacted',
      decline: 'Decline',
      footnote:
        "The seeker's identity is never shown — only the contact they chose to share. You reach out directly.",
    },
  },
  {
    ru: {
      headTitle: 'Рефералы — freehire',
      title: 'Рефералы',
      tabs: {
        requests: 'Мои запросы',
        offers: 'Готов рекомендовать',
        incoming: 'Входящие',
      },
      requests: {
        empty: 'Вы ещё не запрашивали рекомендации.',
        columns: {
          company: 'Компания',
          cvShared: 'Какое CV',
          status: 'Статус',
          sent: 'Отправлен',
        },
        cvBuilt: 'Адаптированное CV',
        cvUploaded: 'Загруженное CV',
        status: {
          sent: 'Отправлен',
          contacted: 'Связались',
          declined: 'Отклонён',
        },
        footnote:
          'Уведомлений здесь нет — рекомендатель свяжется с вами по тому контакту, который вы оставили.',
      },
      offers: {
        lead: 'Компании, куда вы можете рекомендовать.',
        openForm: '+ Готов рекомендовать',
        companyLabel: 'Компания',
        companyHint: 'Найдите и выберите компанию, где вы работаете.',
        linkedinLabel: 'Ваш профиль в LinkedIn',
        linkedinHint: 'Помогает модератору убедиться, что вы там работаете.',
        linkedinInvalid: 'Укажите полную ссылку вида linkedin.com/in/…',
        proofLabel: 'Подтверждение занятости (PDF)',
        proofHint: 'CV или письмо, подтверждающее вашу работу там. Проверит модератор.',
        submit: 'Отправить на проверку',
        submitting: 'Отправляем…',
        empty: 'Вы ещё нигде не предлагали рекомендовать.',
        status: {
          approved: 'Подтверждено',
          pending: 'На проверке',
          rejected: 'Отклонено',
        },
        withdraw: 'Больше не рекомендую',
        withdrawing: 'Удаляем…',
        errors: {
          duplicate: 'Вы уже предложили рекомендовать в эту компанию.',
          unknownCompany: 'Такой компании у нас нет — проверьте слаг в адресе её страницы.',
          badLinkedin: 'Укажите корректную ссылку на профиль LinkedIn.',
          uploadUnavailable: 'Загрузка файлов сейчас недоступна.',
          generic: 'Не удалось отправить заявку. Попробуйте ещё раз.',
        },
        withdrawDialog: {
          titlePrefix: 'Больше не рекомендовать в',
          titleSuffix: '?',
          description: 'Вы сможете предложить снова позже.',
          confirmLabel: 'Больше не рекомендую',
        },
      },
      incoming: {
        empty: 'Входящих запросов на рекомендацию нет.',
        wantsReferralPrefix: 'Кто-то просит рекомендацию в',
        contactLabel: 'Контакт:',
        linkedin: 'LinkedIn ↗',
        viewCv: 'Открыть CV',
        markContacted: 'Связался',
        decline: 'Отклонить',
        footnote:
          'Личность соискателя не показывается — только тот контакт, который он решил оставить. Вы связываетесь напрямую.',
      },
    },
  },
);
