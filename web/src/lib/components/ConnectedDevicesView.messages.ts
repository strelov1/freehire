import { defineMessages } from '$lib/i18n/t';

export const messages = defineMessages(
  {
    title: 'Connected devices',
    intro: 'MCP clients — like Claude, Cursor, or ChatGPT — you have approved to act as your freehire account.',
    errors: {
      confirmFirst: 'Confirm it is you before revoking a connection.',
      loadError: "Couldn't load your connected devices.",
      revokeFailed: 'Could not revoke this connection. Please try again.',
    },
    list: {
      empty: 'No connected devices yet.',
      createdPrefix: 'Connected',
      lastUsedPrefix: 'last used',
      neverUsed: 'never used',
      revoke: 'Revoke',
    },
    revokeDialog: {
      // "Revoke "<client name>"?" — the component supplies the quoted name.
      titlePrefix: 'Revoke',
      titleSuffix: '?',
      description: 'This client will no longer be able to act as your account.',
      confirmPrompt: 'Revoking access is a security change, so we check it is you.',
      confirmLabel: 'Revoke',
    },
  },
  {
    ru: {
      title: 'Подключённые устройства',
      intro: 'MCP-клиенты — например, Claude, Cursor или ChatGPT — которым вы разрешили действовать от имени вашего аккаунта freehire.',
      errors: {
        confirmFirst: 'Подтвердите, что это вы, прежде чем отзывать доступ.',
        loadError: 'Не удалось загрузить список подключённых устройств.',
        revokeFailed: 'Не удалось отозвать доступ. Попробуйте ещё раз.',
      },
      list: {
        empty: 'Пока нет подключённых устройств.',
        createdPrefix: 'Подключено',
        lastUsedPrefix: 'последний раз использован',
        neverUsed: 'ни разу не использован',
        revoke: 'Отозвать',
      },
      revokeDialog: {
        titlePrefix: 'Отозвать',
        titleSuffix: '?',
        description: 'Этот клиент больше не сможет действовать от имени вашего аккаунта.',
        confirmPrompt: 'Отзыв доступа — изменение настроек безопасности, поэтому мы проверяем, что это вы.',
        confirmLabel: 'Отозвать',
      },
    },
  },
);
