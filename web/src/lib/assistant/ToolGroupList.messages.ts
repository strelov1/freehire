import { defineMessages } from '$lib/i18n/t';

// `confirmation.claim`/`confirmation.question` and everything `tool-formatters.ts`
// derives from a call (title/callLine) are the agent's own text or data read from its
// arguments — not catalog text, except `tool-formatters.ts`'s own intent LABELS map,
// which has its own messages file.
export const messages = defineMessages(
  { yes: 'Yes', no: 'No' },
  { ru: { yes: 'Да', no: 'Нет' } },
);
