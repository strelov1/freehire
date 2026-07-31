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
import type { GhostCriterionCode } from './ghost';

export interface SignalExplainer {
  /** The union rather than `string`: the landing hands this straight to the diagram
   *  registry, which is keyed by it. */
  code: GhostCriterionCode;
  /** Short human name — the checklist row's label. */
  label: string;
  /** structural = the shape of the posting; outcome = what happened to an applicant. */
  tier: 'structural' | 'outcome';
  /** An example of the observations behind it — illustrative, not a transcript of the UI. */
  fact: string;
  /** The one-line version, always visible beside the criterion's diagram. Carries the
   *  single limit that matters most for this criterion, because a summary that drops
   *  every caveat is how a hedged page turns into an accusing one. */
  gist: string;
  /** Why this is evidence, and what it is not — the full account, one disclosure away. */
  why: string;
}

const EXPLAINERS: Record<string, { fact: string; gist: string; why: string }> = {
  evergreen_posting: {
    fact: 'Open 240 days · reposted 13× · 7 copies open at once',
    gist: "The same job posted over and over, often with several copies live at once. Age on its own never triggers it — hard senior roles stay open a long time.",
    why: 'One role advertised over and over, often with several copies live at the same time. It never fires on age alone — a genuinely hard senior role stays open a long time. Age is measured from when freehire first saw the posting, not from the date the source prints, so refreshing that date does not reset it.',
  },
  ats_absent: {
    fact: "Not on the company's own careers board · checked 2 days ago",
    gist: "We found the job on an aggregator, but the company's own careers page does not list it. Only counted where we actually check that page.",
    why: "The posting reached us through an aggregator, and the same role is not on the employer's own board. It only counts where we actually crawl that board — otherwise absence would report our blind spot as the employer's fault. The check is re-run continuously and expires if it stops, so a stale answer goes quiet instead of standing.",
  },
  silent_applications: {
    fact: 'Applications through freehire went unanswered past their follow-up window',
    gist: 'People applied through freehire and got no answer in the time their stage allows. Only counted when a mailbox is connected, so a reply would have been seen.',
    why: 'People who applied here, whose mailbox is connected so a reply would have been seen, and who were not answered within the window their stage tolerates. Without a connected mailbox we could not tell silence from a gap in our data, so those applications are not counted at all.',
  },
  user_reports: {
    fact: 'People reported applying with no response',
    gist: "Someone says they applied on a given date and heard nothing back. It carries no weight until enough time has passed, and it can be taken back.",
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

// The gates used to live here and now live in `./ghost`, beside the rule that reads them
// and in the one module that declares itself the mirror of internal/ghost's constants.
// No re-export is left behind: the page no longer quotes the numbers in prose — the gate
// matrix interpolates them from the constants — so every consumer imports them from the
// authority directly.
