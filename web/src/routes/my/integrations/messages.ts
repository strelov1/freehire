import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    title: 'Integrations',
    description: 'Connect your other accounts, and let your own tools reach freehire.',
    tabsLabel: 'Integration sections',
    tabs: { connections: 'Connections', 'api-keys': 'API keys', webhook: 'Webhook' },
  },
  {
    ru: {
      title: 'Интеграции',
      description: 'Подключите другие аккаунты и дайте своим инструментам доступ к freehire.',
      tabsLabel: 'Разделы интеграций',
      tabs: { connections: 'Подключения', 'api-keys': 'API-ключи', webhook: 'Вебхук' },
    },
  },
);
