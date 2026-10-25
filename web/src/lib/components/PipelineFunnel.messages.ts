import { defineMessages } from '#lib/i18n/t.js';

// Band names (`stageGroupLabel`) and stage names (`humanizeStage`) come from the
// generated pipeline-stage vocabulary, translated in `#lib/stages.messages.ts`.
export const messages = defineMessages(
  { chartAria: 'Application pipeline by status' },
  { ru: { chartAria: 'Воронка заявок по статусу' } },
);
