import { defineMessages, plurals } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    headTitle: 'Opening CV… — freehire',
    editorTitle: 'Edit CV — freehire',
    opening: 'Opening your CV…',
    back: 'All CVs',
    tabs: { content: 'Content', design: 'Design' },
    template: 'Template',
    typography: 'Typography',
    margins: 'Page margins',
    marginsUnit: '(inches)',
    save: { saving: 'Saving…', saved: 'Saved', error: 'Could not save. Please try again.' },
    pages: plurals({ one: '{count} page', other: '{count} pages' }),
    pdf: 'Download PDF',
    refresh: 'Update from my CV',
    refreshFailed: 'Could not update this CV from your CV.',
    refreshDialog: {
      title: 'Replace this CV’s content with your current CV?',
      description:
        'Anything you edited by hand in this base CV will be replaced. Your template and layout stay. CVs already tailored from it are not changed.',
      confirm: 'Replace content',
    },
  },
  {
    ru: {
      headTitle: 'Открываем резюме… — freehire',
      editorTitle: 'Редактирование резюме — freehire',
      opening: 'Открываем резюме…',
      back: 'Все резюме',
      tabs: { content: 'Содержимое', design: 'Оформление' },
      template: 'Шаблон',
      typography: 'Типографика',
      margins: 'Поля страницы',
      marginsUnit: '(дюймы)',
      save: { saving: 'Сохранение…', saved: 'Сохранено', error: 'Не удалось сохранить. Попробуйте ещё раз.' },
      pages: plurals({ one: '{count} страница', few: '{count} страницы', many: '{count} страниц', other: '{count} страницы' }),
      pdf: 'Скачать PDF',
      refresh: 'Обновить из моего резюме',
      refreshFailed: 'Не удалось обновить это резюме из вашего резюме.',
      refreshDialog: {
        title: 'Заменить содержимое этого резюме текущим?',
        description:
          'Всё, что вы правили вручную в этом базовом резюме, будет заменено. Шаблон и оформление сохранятся. Уже адаптированные из него копии не изменятся.',
        confirm: 'Заменить',
      },
    },
  },
);
