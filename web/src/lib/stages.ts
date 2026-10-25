// The application-stage vocabulary, in pipeline order (active stages then
// terminal), with its labels and its group membership. All three are generated
// from internal/userjob via cmd/gen-contracts — this module reads them and adds
// no vocabulary of its own. It used to hold a second copy of the labels, which is
// how the board, the funnel and this file came to disagree about what to call a
// settled application. The English labels ARE the generated ones; the translations
// beside them live in `stages.messages.ts`, keyed by the same stage and group ids.

import { STAGE_GROUPS, type Stage } from './generated/contracts';
import { t } from './i18n/t';
import type { Locale } from './locale';
import { messages } from './stages.messages';

interface StageOption {
  value: string;
  label: string;
}

/** One group of stages as the selector renders it: a heading with its options. */
export interface StageOptionGroup {
  id: string;
  label: string;
  options: StageOption[];
}

// Every stage must be placed by the emitted groups, and every stage the groups name must be
// real. Both directions, at compile time: `false extends true` is an error, so a stage added
// to the Go vocabulary without a group fails `pnpm run check` rather than reaching the board
// as a card in no column.
//
// EXPORTED WITH NO CONSUMER, DELIBERATELY. The declaration is the whole check — evaluating
// `Assert<Eq<…>>` is what fails — and `export` is what stops the linter deleting it as an
// unused local. knip reports it as an unused export, correctly and unhelpfully; this is
// the note that says so.
type Assert<T extends true> = T;
type Eq<A, B> = [A] extends [B] ? ([B] extends [A] ? true : false) : false;
type GroupedStage = (typeof STAGE_GROUPS)[number]['stages'][number];
/** @public */
export type StagesAreFullyGrouped = Assert<Eq<Stage, GroupedStage>>;

/** A human label for a stage value in `locale` (English — the generated label — by
 *  default); the value itself, title-cased, when not in the known vocabulary. */
export function humanizeStage(stage: string, locale: Locale = 'en'): string {
  return t(messages, locale).stages[stage] ?? stage.charAt(0).toUpperCase() + stage.slice(1);
}

/** A human label for a stage GROUP id (a board column, a funnel band) in `locale`;
 *  the id itself when not in the known vocabulary. */
export function stageGroupLabel(id: string, locale: Locale = 'en'): string {
  return t(messages, locale).groups[id] ?? id;
}

/** The stages grouped as the selector shows them, in generated pipeline order, so
 *  `Closed` reads as a heading over its three outcomes rather than as a fifth state. */
export function groupedStages(locale: Locale = 'en'): StageOptionGroup[] {
  return STAGE_GROUPS.map((g) => ({
    id: g.id,
    label: stageGroupLabel(g.id, locale),
    options: g.stages.map((value) => ({ value, label: humanizeStage(value, locale) })),
  }));
}

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
