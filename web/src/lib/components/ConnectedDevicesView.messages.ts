import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    title: 'Connected devices',
    intro: 'MCP clients — like Claude, Cursor, or ChatGPT — you have approved to act as your freehire account.',
    agentsMayApply: {
      label: 'Let connected agents apply for me',
      detail:
        'A connected client can start auto-apply, answer the questions an application asks with your saved answers, approve it, and send it to the employer in your name.',
    },
    errors: {
      permissionLoad: "Couldn't load this setting.",
      permissionSave: "Couldn't save this setting. Please try again.",
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
      agentsMayApply: {
        label: 'Разрешить подключённым агентам откликаться за меня',
        detail:
          'Подключённый клиент сможет запускать автоотклик, отвечать на вопросы анкеты вашими сохранёнными ответами, одобрять заявку и отправлять её работодателю от вашего имени.',
      },
      errors: {
        permissionLoad: 'Не удалось загрузить эту настройку.',
        permissionSave: 'Не удалось сохранить настройку. Попробуйте ещё раз.',
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
