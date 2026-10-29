import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    title: 'API plan',
    none: 'An API plan raises the limit for your keys further and licenses the data for use in your product.',
    seePlans: 'See API plans',
    active: '{tier}: {rate} requests a minute for your full-access keys, until {date}.',
    activeNoDate: '{tier}: {rate} requests a minute for your full-access keys.',
    tiers: { api: 'API', api_data: 'Data' },
    rate: 'Your keys are on the {tier} tier: {minute} requests a minute and {day} a day.',
    rateNoCap: 'Your keys are on the {tier} tier: {minute} requests a minute, no daily cap.',
    rateTiers: { free: 'Free', pro: 'Pro', ultra: 'Ultra' },
  },
  {
    ru: {
      title: 'API-тариф',
      none: 'API-тариф ещё сильнее поднимает лимит для ваших ключей и даёт лицензию на использование данных в вашем продукте.',
      seePlans: 'Тарифы API',
      active: '{tier}: {rate} запросов в минуту для ваших ключей с полным доступом, до {date}.',
      activeNoDate: '{tier}: {rate} запросов в минуту для ваших ключей с полным доступом.',
      tiers: { api: 'API', api_data: 'Data' },
      rate: 'Ваши ключи на уровне {tier}: {minute} запросов в минуту и {day} в день.',
      rateNoCap: 'Ваши ключи на уровне {tier}: {minute} запросов в минуту, без дневного лимита.',
      rateTiers: { free: 'Free', pro: 'Pro', ultra: 'Ultra' },
    },
  },
);
