import { defineMessages } from '$lib/i18n/t';

export const messages = defineMessages(
  {
    education: {
      heading: 'Education',
      degreePlaceholder: 'Degree',
      institutionPlaceholder: 'Institution',
      yearPlaceholder: 'Year',
      add: 'Add education',
      empty: 'Nothing here yet — add your education.',
    },
    languages: {
      heading: 'Languages',
      placeholder: 'English\nSpanish',
      empty: 'Nothing here yet — add a language.',
    },
    certifications: {
      heading: 'Certifications',
      placeholder: 'AWS Certified Solutions Architect',
      empty: 'Nothing here yet — add a certification.',
    },
    edit: 'Edit',
    remove: 'Remove',
    save: 'Save',
    cancel: 'Cancel',
    saveFailed: 'Could not save.',
  },
  {
    ru: {
      education: {
        heading: 'Образование',
        degreePlaceholder: 'Степень',
        institutionPlaceholder: 'Учебное заведение',
        yearPlaceholder: 'Год',
        add: 'Добавить образование',
        empty: 'Пока пусто — добавьте образование.',
      },
      languages: {
        heading: 'Языки',
        placeholder: 'Английский\nИспанский',
        empty: 'Пока пусто — добавьте язык.',
      },
      certifications: {
        heading: 'Сертификаты',
        placeholder: 'AWS Certified Solutions Architect',
        empty: 'Пока пусто — добавьте сертификат.',
      },
      edit: 'Изменить',
      remove: 'Удалить',
      save: 'Сохранить',
      cancel: 'Отмена',
      saveFailed: 'Не удалось сохранить.',
    },
  },
);
