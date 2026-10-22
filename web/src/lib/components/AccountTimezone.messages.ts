import { defineMessages } from '#lib/i18n/t.js';

// IANA zone identifiers themselves (e.g. `Europe/Moscow`) are not catalog keys —
// they're stable wire values Go's time.LoadLocation reads back, not prose.
export const messages = defineMessages(
  {
    heading: 'Timezone',
    description:
      'Used to schedule a daily search-alert digest and quiet hours at your own local time.',
    saving: 'Saving…',
    saved: 'Saved',
    saveFailed: 'Could not save.',
    selectPlaceholder: 'Select a timezone',
  },
  {
    ru: {
      heading: 'Часовой пояс',
      description:
        'Используется, чтобы присылать дайджест по поисковым алертам и учитывать тихие часы по вашему местному времени.',
      saving: 'Сохраняем…',
      saved: 'Сохранено',
      saveFailed: 'Не удалось сохранить.',
      selectPlaceholder: 'Выберите часовой пояс',
    },
  },
);
