import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    heading: 'Your contacts',
    description:
      'Edit without re-uploading. A new CV parse only fills empty fields — it will not overwrite what you typed.',
    fullNamePlaceholder: 'Full name',
    emailPlaceholder: 'Email',
    phonePlaceholder: 'Phone',
    locationPlaceholder: 'Location',
    linksLabel: 'Links (one per line)',
    linksPlaceholder: 'https://…',
    invalidEmail: "That doesn't look like an email address. It isn't saved until it does.",
  },
  {
    ru: {
      heading: 'Ваши контакты',
      description:
        'Редактируйте без повторной загрузки. Новый разбор резюме заполняет только пустые поля — он не перезапишет то, что вы ввели.',
      fullNamePlaceholder: 'Полное имя',
      emailPlaceholder: 'Email',
      phonePlaceholder: 'Телефон',
      locationPlaceholder: 'Местоположение',
      linksLabel: 'Ссылки (по одной на строку)',
      linksPlaceholder: 'https://…',
      invalidEmail: 'Это не похоже на адрес почты. Он не сохранится, пока не будет исправлен.',
    },
  },
);
