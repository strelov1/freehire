// The application-stage vocabulary, in pipeline order (active stages then
// terminal). STAGE_VALUES is generated from the Go userjob.Stages slice in
// internal/userjob via cmd/gen-contracts. Drift is not fatal: humanizeStage
// renders an unknown value as a readable label.

import { STAGE_VALUES } from './generated/contracts';

export interface StageOption {
  value: string;
  label: string;
}

const STAGE_LABELS: Record<string, string> = {
  applied: 'Applied',
  screening: 'Screening',
  responded: 'Responded',
  interview: 'Interview',
  offer: 'Offer',
  accepted: 'Accepted',
  rejected: 'Rejected',
  withdrawn: 'Withdrawn',
};

/** A human label for a stage value; the value itself (title-cased fallback) when
 *  not in the known vocabulary. */
export function humanizeStage(stage: string): string {
  return STAGE_LABELS[stage] ?? stage.charAt(0).toUpperCase() + stage.slice(1);
}

export const STAGES: StageOption[] = STAGE_VALUES.map((value) => ({
  value,
  label: STAGE_LABELS[value] ?? humanizeStage(value),
}));

/** The stages from which an interview has plausibly already happened, and so a debrief
 *  is worth offering.
 *
 *  `rejected` is in the set on purpose: a rejection that arrived after an interview is
 *  where the review is worth the most, and hiding the offer from the candidate with the
 *  strongest reason to use it costs more than a button that occasionally sits on an
 *  application rejected at the screen. `withdrawn` is not — nobody reviews an interview
 *  they walked away from before it happened.
 *
 *  This is the client's judgement about where to advertise the conversation, not a rule:
 *  the backend admits a debrief for an application in any stage, because someone who sat
 *  an interview and never moved their stage is exactly who it is for. */
const DEBRIEFABLE_STAGES = new Set(['interview', 'offer', 'accepted', 'rejected']);

export function offersDebrief(stage: string): boolean {
  return DEBRIEFABLE_STAGES.has(stage);
}
