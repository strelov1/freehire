import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    heading: 'Summary',
    headlineLabel: 'Headline',
    headlinePlaceholder: 'e.g. Staff Backend Engineer',
    summaryPlaceholder: 'A short professional summary…',
    cvLocation: 'As stated on your CV: {location}',
  },
  {
    ru: {
      heading: 'Резюме',
      headlineLabel: 'Заголовок',
      headlinePlaceholder: 'например, Senior Backend Engineer',
      summaryPlaceholder: 'Краткое профессиональное резюме…',
      cvLocation: 'Как указано в резюме: {location}',
    },
  },
);
