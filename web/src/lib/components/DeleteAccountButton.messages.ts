import { defineMessages } from '$lib/i18n/t';

// `erased` is named concretely rather than as a vague "your data" — keep each
// item specific when translating too.
export const messages = defineMessages(
  {
    trigger: 'Delete account',
    dialogTitle: 'Delete account',
    warning:
      'This permanently erases your account and everything in it. It cannot be undone, and we cannot restore any of it afterwards.',
    erased: [
      'Your CV, its parsed profile and every CV you built or tailored',
      'Your hosted mailbox, connected Gmail and all stored messages',
      'Saved jobs, applications, reminders and match analyses',
      'Your plan and its usage history, saved searches, filters and API keys',
      'Your anonymous community handle',
    ],
    discussionsNote:
      "Discussions you started stay up so other members don't lose their replies, but your handle is removed from them — they are shown as written by a deleted member.",
    subscriptionNote:
      'If you pay for Pro, deleting your account here does not cancel that subscription — cancel it first, or you will keep being charged for an account that no longer exists.',
    manageSubscription: 'Manage subscription',
    confirmPrefix: 'Type',
    confirmSuffix: 'to confirm',
    passwordLabel: 'Confirm your password',
    passwordRequired: 'Enter your password to confirm.',
    wrongPassword: 'That password is not right.',
    reauthRequired: 'Confirm your identity before deleting your account.',
    // Split around the provider name for the same reason the email confirmation is:
    // catalog leaves are plain strings, so the component composes the sentence.
    reauthWithPrefix: 'Confirm with',
    reauthProvidersLoading: 'Loading connected providers…',
    reauthProvidersError: 'Could not load connected providers.',
    cancel: 'Cancel',
    deleting: 'Deleting…',
    confirmDelete: 'Delete account permanently',
    genericError: 'Could not delete your account. Nothing was deleted — please try again.',
  },
  {
    trigger: 'Удалить аккаунт',
    dialogTitle: 'Удалить аккаунт',
    warning:
      'Это безвозвратно удалит ваш аккаунт и всё, что с ним связано. Это действие нельзя отменить, и восстановить данные будет невозможно.',
    erased: [
      'Ваше CV, его разобранный профиль и все CV, которые вы создали или адаптировали',
      'Ваш почтовый ящик freehire, подключённый Gmail и все сохранённые сообщения',
      'Сохранённые вакансии, отклики, напоминания и анализы соответствия',
      'Тариф и история его расхода, сохранённые поиски, фильтры и API-ключи',
      'Ваш анонимный ник в сообществе',
    ],
    discussionsNote:
      'Начатые вами обсуждения останутся видны, чтобы другие участники не потеряли свои ответы, но ваш ник будет из них убран — они будут показаны как написанные удалённым участником.',
    subscriptionNote:
      'Если у вас оплачен Pro, удаление аккаунта здесь не отменяет подписку — сначала отмените её, иначе списания продолжатся за уже несуществующий аккаунт.',
    manageSubscription: 'Управление подпиской',
    confirmPrefix: 'Введите',
    confirmSuffix: 'для подтверждения',
    passwordLabel: 'Подтвердите пароль',
    passwordRequired: 'Введите пароль для подтверждения.',
    wrongPassword: 'Неверный пароль.',
    reauthRequired: 'Подтвердите свою личность, прежде чем удалять аккаунт.',
    reauthWithPrefix: 'Подтвердить через',
    reauthProvidersLoading: 'Загрузка подключённых провайдеров…',
    reauthProvidersError: 'Не удалось загрузить подключённых провайдеров.',
    cancel: 'Отмена',
    deleting: 'Удаление…',
    confirmDelete: 'Удалить аккаунт навсегда',
    genericError: 'Не удалось удалить аккаунт. Ничего не было удалено — попробуйте ещё раз.',
  },
);
