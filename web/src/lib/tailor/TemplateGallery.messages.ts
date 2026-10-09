import { defineMessages } from '$lib/i18n/t';

// `t.label` and `t.style` (the template's own name and style description) are
// server data from `api.listCvTemplates()`, not catalog text.
export const messages = defineMessages(
  {
    loadingTemplates: 'Loading templates…',
    loadFailed: 'Could not load templates.',
    switchFailed: 'Could not switch template.',
    previewAlt: '{label} template preview',
    photoNudge: 'Add a photo in your profile — this template shows one',
  },
  {
    ru: {
      loadingTemplates: 'Загружаем шаблоны…',
      loadFailed: 'Не удалось загрузить шаблоны.',
      switchFailed: 'Не удалось сменить шаблон.',
      previewAlt: 'Превью шаблона «{label}»',
      photoNudge: 'Добавьте фото в профиль — этот шаблон его показывает',
    },
  },
);
