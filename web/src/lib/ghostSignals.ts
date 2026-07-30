// The explanation of each ghost criterion, for the /features/ghost-jobs landing.
//
// Keyed by the SAME codes internal/ghost emits and $lib/ghost renders, so the page
// cannot fall behind the product: ghostSignals.test.ts fails if a criterion joins the
// vocabulary without an explanation here. A marketing page a test keeps honest — the
// same trick the inbox landing uses for its status list.
//
// `fact` is an example of the observations that make a criterion fire; `why` is what
// makes it evidence. `fact` is deliberately NOT a transcript of the interface — the job
// page prints only what the payload carries, which for `evergreen_posting` is nothing
// beyond the tick. Both are written from the code, not from the pitch: nothing here
// claims the system knows an employer's intent, because it does not.

import { CRITERIA } from './ghost';

export interface SignalExplainer {
  code: string;
  /** Short human name — the checklist row's label. */
  label: string;
  /** structural = the shape of the posting; outcome = what happened to an applicant. */
  tier: 'structural' | 'outcome';
  /** An example of the observations behind it — illustrative, not a transcript of the UI. */
  fact: string;
  /** Why this is evidence, and what it is not. */
  why: string;
}

const EXPLAINERS: Record<string, { fact: string; why: string }> = {
  evergreen_posting: {
    fact: 'Open 240 days · reposted 13× · 7 copies open at once',
    why: 'One role advertised over and over, often with several copies live at the same time. It never fires on age alone — a genuinely hard senior role stays open a long time. Age is measured from when freehire first saw the posting, not from the date the source prints, so refreshing that date does not reset it.',
  },
  ats_absent: {
    fact: "Not on the company's own careers board · checked 2 days ago",
    why: "The posting reached us through an aggregator, and the same role is not on the employer's own board. It only counts where we actually crawl that board — otherwise absence would report our blind spot as the employer's fault. The check is re-run continuously and expires if it stops, so a stale answer goes quiet instead of standing.",
  },
  silent_applications: {
    fact: 'Applications through freehire went unanswered past their follow-up window',
    why: 'People who applied here, whose mailbox is connected so a reply would have been seen, and who were not answered within the window their stage tolerates. Without a connected mailbox we could not tell silence from a gap in our data, so those applications are not counted at all.',
  },
  user_reports: {
    fact: 'People reported applying with no response',
    why: 'Someone states they applied on a given date and heard nothing. It carries no weight until the same span has passed that a tracked application gets, and it can be withdrawn — an employer who answers late costs the posting its evidence.',
  },
};

/** The criteria with their explanations, in the order the classifier reports them.
 *  A criterion with no explanation is dropped rather than rendered half-empty — and
 *  ghostSignals.test.ts fails on the gap, so it cannot go unnoticed. */
export const GHOST_SIGNALS: SignalExplainer[] = CRITERIA.flatMap((c) => {
  const explainer = EXPLAINERS[c.code];
  return explainer ? [{ code: c.code, label: c.label, tier: c.tier, ...explainer }] : [];
});

/** How many criteria must fire before anything is shown. Mirrors internal/ghost. */
export const CONVERGENCE = 2;

/** How many distinct people must have contributed outcome evidence for the stronger
 *  claim — and the reason a count is never shown below it. */
export const WITNESS_GATE = 2;
