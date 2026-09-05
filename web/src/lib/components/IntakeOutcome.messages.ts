import { defineMessages } from '$lib/i18n/t';

// Only the two links. The outcome sentence itself lives in `$lib/intakeOutcome`,
// which both surfaces share and which is tested on its own.
export const messages = defineMessages(
  {
    viewTheJob: 'View the job →',
    viewTheCompany: 'View the company →',
  },
  {
    ru: {
      viewTheJob: 'Открыть вакансию →',
      viewTheCompany: 'Открыть компанию →',
    },
  },
);
