import { defineMessages } from '$lib/i18n/t';

// `b.label`/`r.label` (band names) and `humanizeStage` come from the generated
// pipeline-stage vocabulary — out of scope here, same as PipelineView.messages.ts.
export const messages = defineMessages(
  { chartAria: 'Application pipeline by status' },
  { ru: { chartAria: 'Воронка заявок по статусу' } },
);
