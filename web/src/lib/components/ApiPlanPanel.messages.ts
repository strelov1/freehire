import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    title: 'API plan',
    none: 'An API plan licenses the data for use in your product.',
    seePlans: 'See API plans',
    active: '{tier} plan active until {date}.',
    activeNoDate: '{tier} plan active.',
    tiers: { api: 'API', api_data: 'Data' },
  },
  {
    ru: {
      title: 'API-тариф',
      none: 'API-тариф даёт лицензию на использование данных в вашем продукте.',
      seePlans: 'Тарифы API',
      active: 'Тариф {tier} действует до {date}.',
      activeNoDate: 'Тариф {tier} активен.',
      tiers: { api: 'API', api_data: 'Data' },
    },
  },
);
