import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    title: 'New API key',
    nameLabel: 'Name',
    namePlaceholder: 'e.g. CI bot',
    expiryLabel: 'Expiry',
    expiryNever: 'No expiry',
    expiry30: '30 days',
    expiry90: '90 days',
    expiry365: '1 year',
    scopeLabel: 'Scope',
    scopeFull: 'Full access',
    scopeEmbed: 'Embed only (volunteer embedding)',
    scopeHint:
      'Embed-only keys can only lease and submit volunteer embedding tasks — nothing else on your account.',
    // The `prompt` handed to ConfirmIdentity: why THIS action is worth confirming. A key is
    // a bearer credential with no session behind it, which is a different danger from the
    // one account deletion carries, so the sentence lives with the action.
    confirmPrompt: 'An API key is standing access to your account, so we check it is you.',
    cancel: 'Cancel',
    create: 'Create key',
    creating: 'Creating…',
    errors: {
      nameRequired: 'Give the key a name.',
      createFailed: 'Could not create the key. Please try again.',
    },
  },
  {
    ru: {
      title: 'Новый API-ключ',
      nameLabel: 'Название',
      namePlaceholder: 'например, CI bot',
      expiryLabel: 'Срок действия',
      expiryNever: 'Бессрочно',
      expiry30: '30 дней',
      expiry90: '90 дней',
      expiry365: '1 год',
      scopeLabel: 'Доступ',
      scopeFull: 'Полный доступ',
      scopeEmbed: 'Только эмбеддинг (волонтёрский)',
      scopeHint:
        'Ключи с доступом «только эмбеддинг» могут только брать и сдавать волонтёрские задачи эмбеддинга — больше ничего на вашем аккаунте.',
      confirmPrompt: 'API-ключ — это постоянный доступ к аккаунту, поэтому мы проверяем, что это вы.',
      cancel: 'Отмена',
      create: 'Создать ключ',
      creating: 'Создание…',
      errors: {
        nameRequired: 'Дайте ключу название.',
        createFailed: 'Не удалось создать ключ. Попробуйте ещё раз.',
      },
    },
  },
);
