import { defineMessages } from '$lib/i18n/t';

// Only the tab-strip labels — the views themselves (ProfileForm,
// ExperienceBankView, etc.) are out of scope for this pass.
export const messages = defineMessages(
  {
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
