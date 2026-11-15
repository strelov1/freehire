import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    headTitle: 'Join a company team — freehire',
    title: 'Join {company} on freehire',
    body: 'You were invited at {email}. You will act for {company} under that address: see and move the applications to its vacancies, and publish vacancies of your own.',
    join: 'Join the team',
    joining: 'Joining…',
    dead: 'This invitation link is invalid or has expired. Ask your colleague to send a new one.',
    previewFailed: 'Could not load this invitation. Check your connection and try again.',
    retry: 'Try again',
    noToken: 'This page opens from an invitation email.',
    alreadyMember: 'You already act for a company on freehire, and an account acts for one company only.',
    error: 'Something went wrong. Please try again.',
  },
  {
    ru: {
      headTitle: 'Вступить в команду компании — freehire',
      title: 'Вступить в команду {company} на freehire',
      body: 'Вас пригласили на адрес {email}. Вы будете действовать от имени {company} под этим адресом: видеть отклики на её вакансии, вести их по стадиям и публиковать свои вакансии.',
      join: 'Вступить в команду',
      joining: 'Вступаем…',
      dead: 'Ссылка-приглашение недействительна или устарела. Попросите коллегу прислать новую.',
      previewFailed: 'Не удалось загрузить приглашение. Проверьте подключение и попробуйте ещё раз.',
      retry: 'Попробовать ещё раз',
      noToken: 'Эта страница открывается по ссылке из письма-приглашения.',
      alreadyMember:
        'Вы уже действуете от имени компании на freehire, а один аккаунт может представлять только одну компанию.',
      error: 'Что-то пошло не так. Попробуйте ещё раз.',
    },
  },
);
