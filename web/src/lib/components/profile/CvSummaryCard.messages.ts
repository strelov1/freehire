import { defineMessages } from '$lib/i18n/t';

export const messages = defineMessages(
  {
    heading: 'Summary',
    edit: 'Edit',
    headlineLabel: 'Headline',
    headlinePlaceholder: 'e.g. Staff Backend Engineer',
    summaryPlaceholder: 'A short professional summary…',
    save: 'Save',
    cancel: 'Cancel',
    saveFailed: 'Could not save.',
    cvLocation: 'As stated on your CV: {location}',
    empty: 'Nothing here yet — add a headline or summary.',
  },
  {
    ru: {
      heading: 'Резюме',
      edit: 'Изменить',
      headlineLabel: 'Заголовок',
      headlinePlaceholder: 'например, Senior Backend Engineer',
      summaryPlaceholder: 'Краткое профессиональное резюме…',
      save: 'Сохранить',
      cancel: 'Отмена',
      saveFailed: 'Не удалось сохранить.',
      cvLocation: 'Как указано в резюме: {location}',
      empty: 'Пока пусто — добавьте заголовок или краткое описание.',
    },
  },
);
