import { defineMessages } from '$lib/i18n/t';

// The section heading and the tab-strip labels — the views themselves (ProfileForm,
// ExperienceBankView, etc.) are out of scope for this pass.
export const messages = defineMessages(
  {
    title: 'Profile',
    description: 'Your CV, skills and role — measured against live market demand.',
    tabs: {
      profile: 'Profile',
      contacts: 'Contacts',
      location: 'Location',
      skills: 'Skills',
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
      tabs: {
        profile: 'Профиль',
        contacts: 'Контакты',
        location: 'Локация',
        skills: 'Навыки',
        experience: 'Опыт',
        education: 'Образование',
        screening: 'Ответы на вопросы',
        settings: 'Настройки',
      },
    },
  },
);
