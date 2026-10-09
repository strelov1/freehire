import { defineMessages } from '$lib/i18n/t';

// `entry.note`/`entry.whyFits`/`entry.concerns` are the agent's own rationale text —
// not catalog text. `JobRow` itself stays untranslated (the known, documented gap
// from `i18n-my-account-fanout`); this covers only the fetch-failure fallback card.
export const messages = defineMessages(
  { viewJob: 'View job' },
  { ru: { viewJob: 'Открыть вакансию' } },
);
