import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    title: 'API plan',
    none: 'Without a plan, requests are limited per address like any visitor’s. A plan raises the limit for your keys and licenses the data for use in your product.',
    seePlans: 'See API plans',
    active: '{tier}: {rate} requests a minute for your full-access keys, until {date}.',
    activeNoDate: '{tier}: {rate} requests a minute for your full-access keys.',
    tiers: { api: 'API', api_data: 'Data' },
  },
  {
    ru: {
      title: 'API-тариф',
      none: 'Без тарифа запросы ограничены по адресу, как у любого посетителя. Тариф поднимает лимит для ваших ключей и даёт лицензию на использование данных в вашем продукте.',
      seePlans: 'Тарифы API',
      active: '{tier}: {rate} запросов в минуту для ваших ключей с полным доступом, до {date}.',
      activeNoDate: '{tier}: {rate} запросов в минуту для ваших ключей с полным доступом.',
      tiers: { api: 'API', api_data: 'Data' },
    },
  },
);
