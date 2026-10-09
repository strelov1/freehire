import { defineMessages } from '$lib/i18n/t';

export const messages = defineMessages(
  {
    skillsHeading: 'Skills to avoid',
    skillsPlaceholder: 'Search skills to exclude',
    sourcesHeading: 'Sources to avoid',
    sourcesPlaceholder: 'Search sources to exclude',
    companiesHeading: 'Companies to avoid',
    companiesPlaceholder: 'Search companies to exclude',
    updateFailed: 'Could not update {value} in your profile. Try again.',
  },
  {
    ru: {
      skillsHeading: 'Навыки для исключения',
      skillsPlaceholder: 'Найдите навык, чтобы исключить',
      sourcesHeading: 'Источники для исключения',
      sourcesPlaceholder: 'Найдите источник, чтобы исключить',
      companiesHeading: 'Компании для исключения',
      companiesPlaceholder: 'Найдите компанию, чтобы исключить',
      updateFailed: 'Не удалось обновить «{value}» в профиле. Попробуйте ещё раз.',
    },
  },
);
