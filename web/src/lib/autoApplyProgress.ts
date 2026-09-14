// Maps auto-apply's seven-value status (see autoApplyReview.ts) plus one frontend-only
// signal — whether the ledger already shows a successful auto-apply submission for this
// job (see openspec/changes/auto-apply-progress-tab/design.md) — onto the tracker
// drawer's 3-stage progress view: Tailoring, Review, Submitted.
//
// One function over the whole status, not three independent per-stage checks, because a
// later stage's state always depends on how an earlier one resolved (e.g. `blocked` only
// makes sense once Tailoring and Review are both already `done`).

export type AutoApplyProgressStepId = 'tailoring' | 'review' | 'submitted';
type AutoApplyProgressStepState = 'pending' | 'active' | 'done' | 'queued' | 'error';

export interface AutoApplyProgressStep {
  id: AutoApplyProgressStepId;
  state: AutoApplyProgressStepState;
}

function steps(
  tailoring: AutoApplyProgressStepState,
  review: AutoApplyProgressStepState,
  submitted: AutoApplyProgressStepState
): AutoApplyProgressStep[] {
  return [
    { id: 'tailoring', state: tailoring },
    { id: 'review', state: review },
    { id: 'submitted', state: submitted },
  ];
}

/** Builds the 3-stage progress view, or null when there is nothing to show — no live
 *  attempt and no confirmed auto-apply submission on record. */
export function autoApplyProgressSteps(
  status: string | null | undefined,
  autoApplied: boolean
): AutoApplyProgressStep[] | null {
  switch (status) {
    case 'tailoring':
      return steps('active', 'pending', 'pending');
    case 'tailor_failed':
      return steps('error', 'pending', 'pending');
    case 'pending_review':
      return steps('done', 'active', 'pending');
    case 'declined':
      return steps('done', 'error', 'pending');
    case 'approved':
      return steps('done', 'done', 'queued');
    case 'blocked':
    case 'failed':
      return steps('done', 'done', 'error');
    default:
      return autoApplied ? steps('done', 'done', 'done') : null;
  }
}
