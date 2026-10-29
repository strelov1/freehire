import { defineMessages } from '#lib/i18n/t.js';

// The section heading, the tab-strip labels, the headings of the parts that used to be
// tabs of their own, and the load gate's failure — the views themselves (ProfileForm,
// ExperienceBankView, etc.) are out of scope for this pass.
export const messages = defineMessages(
  {
    title: 'Profile',
    description: 'Your CV, skills and role — measured against live market demand.',
    loadError: "Couldn't load your profile.",
    completeness: {
      summary: '{filled}/{total} sections',
    },
    tabs: {
      profile: 'Profile',
      skills: 'Skills',
      experience: 'Experience',
      screening: 'Screening answers',
    },
    headings: {
      location: 'Location',
      avoid: 'Avoid',
    },
  },
  {
    ru: {
      title: 'Профиль',
      description: 'Ваше резюме, навыки и роль — в сравнении с живым спросом рынка.',
      loadError: 'Не удалось загрузить профиль.',
      completeness: {
        summary: 'Заполнено: {filled}/{total}',
      },
      tabs: {
        profile: 'Профиль',
        skills: 'Навыки',
        experience: 'Опыт',
        screening: 'Ответы на вопросы',
      },
      headings: {
        location: 'Локация',
        avoid: 'Исключения',
      },
    },
  },
);
