import { defineMessages } from '$lib/i18n/t';

// `lead` is a prop supplied by each page (template/typography), translated by that
// page's own catalog — only the part shared by both panes lives here.
export const messages = defineMessages(
  {
    leadSuffix:
      'Changes here only affect CVs you create from now on — CVs you already have keep their own appearance.',
    loadFailed: 'Could not load your appearance defaults.',
    loading: 'Loading…',
    saveDefaults: 'Save defaults',
    saved: 'Saved.',
  },
  {
    ru: {
      leadSuffix:
        'Изменения касаются только новых резюме — уже созданные сохраняют собственное оформление.',
      loadFailed: 'Не удалось загрузить настройки оформления по умолчанию.',
      loading: 'Загрузка…',
      saveDefaults: 'Сохранить настройки',
      saved: 'Сохранено.',
    },
  },
);
