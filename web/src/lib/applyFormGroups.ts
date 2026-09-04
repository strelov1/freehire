// What the job page reads out of a captured application form: the employer's
// questions partitioned by what answering one costs, cheapest kind first.
//
// A plain module rather than a `$derived` inside JobApplyForm.svelte, because the
// web suite runs in plain Node with no Svelte compilation (see vitest.config.ts):
// logic written in a `.svelte` file is logic no test in this repo can reach, and
// the counting and the collapse rules are exactly the parts worth pinning.
import type { Question } from './generated/contracts';

/** The kinds of answer a question can expect, ordered by what answering costs —
 *  which is the order the groups are shown in, and the whole point of grouping. */
const GROUP_ORDER = ['short', 'choice', 'written', 'upload'] as const;

export type GroupKey = (typeof GROUP_ORDER)[number];

/** Maps the served `answer` vocabulary onto the groups. The vocabulary is closed and
 *  authored in internal/ingest/applyform/display.go; the empty string is a member of
 *  it rather than a gap — that projection deliberately names no kind for a one-line
 *  answer, since naming what everyone assumes is noise.
 *
 *  It also names no kind for a control it could not normalize, so an unrecognized
 *  word resolves the same way an absent one does: to the assumption a reader makes
 *  of any unqualified question. */
const GROUP_BY_ANSWER: Record<string, GroupKey> = {
  '': 'short',
  'choose one': 'choice',
  'choose any': 'choice',
  'yes / no': 'choice',
  'written answer': 'written',
  upload: 'upload',
};

/** Not exported: nothing outside needs to name it. A caller reaches a group through
 *  `ApplyFormModel.groups` and gets the same type structurally. */
interface QuestionGroup {
  key: GroupKey;
  questions: Question[];
}

export interface ApplyFormModel {
  /** The non-empty groups, cheapest kind first. */
  groups: QuestionGroup[];
  /** How many questions the employer asks. */
  total: number;
  /** How many of them demand a written answer — the figure that decides whether
   *  applying costs a minute or an evening. A zero is counted honestly here and
   *  simply not printed; withholding it would say "we do not know" instead. */
  written: number;
  /** Whether the groups are worth heading. */
  headed: boolean;
}

/** The kinds a lone group needs no heading for, because the heading would be the
 *  second place the kind is named rather than the only one: `written` is already
 *  counted in the summary above, and `short` was never named anywhere — display.go
 *  withholds the word for a one-line answer on purpose.
 *
 *  Every other kind is named ONLY by its heading, since the hint that used to sit on
 *  each row moved there. Suppressing it would delete the fact, and a form asking
 *  nothing but two work-authorization dropdowns is an ordinary form to be handed. */
const NEEDS_NO_HEADING: readonly GroupKey[] = ['short', 'written'];

const groupOf = (question: Question): GroupKey => GROUP_BY_ANSWER[question.answer ?? ''] ?? 'short';

export function applyFormModel(questions: Question[]): ApplyFormModel {
  // Walked once per group rather than bucketed in one pass: the order of the groups
  // and the order within each of them both fall out of the walk, and a form holds a
  // few dozen questions at most.
  const groups = GROUP_ORDER.map((key) => ({
    key,
    questions: questions.filter((question) => groupOf(question) === key),
  })).filter((group) => group.questions.length > 0);

  const [only] = groups;

  return {
    groups,
    total: questions.length,
    written: groups.find((group) => group.key === 'written')?.questions.length ?? 0,
    headed: groups.length > 1 || (only !== undefined && !NEEDS_NO_HEADING.includes(only.key)),
  };
}
