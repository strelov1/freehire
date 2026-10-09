import { defineMessages } from '$lib/i18n/t';

export const messages = defineMessages(
  {
    defaultPlaceholder: 'Notes…',
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
