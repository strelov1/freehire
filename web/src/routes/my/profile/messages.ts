import { defineMessages } from '#lib/i18n/t.js';

// The section heading, the tab-strip labels and the load gate's failure — the views themselves (ProfileForm,
// ExperienceBankView, etc.) are out of scope for this pass.
export const messages = defineMessages(
  {
    title: 'Profile',
    description: 'Your CV, skills and role — measured against live market demand.',
    loadError: "Couldn't load your profile.",
    tabs: {
      profile: 'Profile',
      contacts: 'Contacts',
      location: 'Location',
      skills: 'Skills',
      avoid: 'Avoid',
      experience: 'Experience',
      education: 'Education',
      screening: 'Screening answers',
      settings: 'Settings',
    },
  },
  {
    ru: {
      title: 'Профиль',
      description: 'Ваше резюме, навыки и роль — в сравнении с живым спросом рынка.',
      loadError: 'Не удалось загрузить профиль.',
      tabs: {
        profile: 'Профиль',
        contacts: 'Контакты',
        location: 'Локация',
        skills: 'Навыки',
        avoid: 'Исключения',
        experience: 'Опыт',
        education: 'Образование',
        screening: 'Ответы на вопросы',
        settings: 'Настройки',
      },
    },
  },
);
