import { defineMessages } from '$lib/i18n/t';

// `fmtPct` formats the number itself and stays locale-invariant; only the "New"
// badge (shown when there is no prior period to compare against) is prose.
export const messages = defineMessages(
  {
    new: 'New',
  },
  {
    ru: {
      new: 'Новое',
    },
  },
);
