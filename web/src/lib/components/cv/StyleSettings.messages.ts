import { defineMessages } from '$lib/i18n/t';

// `f.label`/`f.note` (the font list from the API) are server data, not catalog text.
export const messages = defineMessages(
  {
    font: 'Font',
    templateDefault: 'Template default',
    fontSize: 'Font size',
    fontSizeHintDefault: 'From the template',
    fontSizeHintPoints: 'points',
    lineHeight: 'Line height',
    lineHeights: {
      compact: 'Compact',
      standard: 'Standard',
      relaxed: 'Relaxed',
      loose: 'Loose',
    },
    custom: 'Custom ({value})',
    resetToDefault: 'Reset to template default',
  },
  {
    ru: {
      font: 'Шрифт',
      templateDefault: 'По умолчанию из шаблона',
      fontSize: 'Размер шрифта',
      fontSizeHintDefault: 'Из шаблона',
      fontSizeHintPoints: 'пункты',
      lineHeight: 'Межстрочный интервал',
      lineHeights: {
        compact: 'Плотный',
        standard: 'Обычный',
        relaxed: 'Свободный',
        loose: 'Широкий',
      },
      custom: 'Особый ({value})',
      resetToDefault: 'Вернуть значения шаблона',
    },
  },
);
