import { defineMessages } from '$lib/i18n/t';

// `status` mirrors the API's `Submission['status']` enum, which this view renders
// verbatim as the pill's label. Translating it means mapping it here rather than
// printing the wire value — the enum is a protocol token, not display text.
export const messages = defineMessages(
  {
    headTitle: 'My submissions — freehire',
    signedOut: 'Sign in to see your submissions.',
    title: 'My submissions',
    // Split around the link, which sits mid-sentence.
    descriptionPrefix: 'Jobs you submitted for review.',
    submitAnother: 'Submit another',
    loadError: "Couldn't load your submissions.",
    empty: 'No submissions yet. Submit a job to see it here.',
    // The meta line reads "<company> · <location> · submitted <2 days ago>".
    submittedPrefix: 'submitted',
    rejectionReasonPrefix: 'Reason:',
    viewVacancy: 'View vacancy →',
    status: {
      pending: 'pending',
      approved: 'approved',
      rejected: 'rejected',
    },
  },
  {
    ru: {
      headTitle: 'Мои вакансии — freehire',
      signedOut: 'Войдите, чтобы увидеть отправленные вакансии.',
      title: 'Мои вакансии',
      descriptionPrefix: 'Вакансии, которые вы отправили на проверку.',
      submitAnother: 'Отправить ещё',
      loadError: 'Не удалось загрузить ваши вакансии.',
      empty: 'Пока ничего нет. Отправьте вакансию — она появится здесь.',
      submittedPrefix: 'отправлено',
      rejectionReasonPrefix: 'Причина:',
      viewVacancy: 'Открыть вакансию →',
      status: {
        pending: 'на проверке',
        approved: 'принята',
        rejected: 'отклонена',
      },
    },
  },
);
