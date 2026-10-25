import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    defaultPlaceholder: 'Notes…',
    saving: 'Saving…',
    saved: 'Saved',
    saveFailed: "Couldn't save — your text is still here.",
    retry: 'Retry',
    toolbar: {
      bold: 'Bold',
      italic: 'Italic',
      heading: 'Heading',
      unorderedList: 'Bulleted list',
      orderedList: 'Numbered list',
      link: 'Create link',
      preview: 'Toggle preview',
    },
  },
  {
    ru: {
      defaultPlaceholder: 'Заметки…',
      saving: 'Сохраняем…',
      saved: 'Сохранено',
      saveFailed: 'Не удалось сохранить — текст остался здесь.',
      retry: 'Повторить',
      toolbar: {
        bold: 'Жирный',
        italic: 'Курсив',
        heading: 'Заголовок',
        unorderedList: 'Маркированный список',
        orderedList: 'Нумерованный список',
        link: 'Вставить ссылку',
        preview: 'Предпросмотр',
      },
    },
  },
);
