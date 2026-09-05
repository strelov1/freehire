// Maps auto-apply's six-value status (jobtracking/AssembleReviewInfo, openspec/changes/
// auto-apply-review-tracking) to the tracker's two rendering decisions — kept out of
// BoardCard.svelte/JobDrawer.svelte so both unit-test without mounting Svelte, mirroring
// autoApplyButton.ts's own convention.

/** Whether the board card shows its "needs your review" badge — an entry the candidate
 *  must act on (pending_review) or one that stopped and needs their attention (blocked).
 *  `tailoring`, `approved` and terminal `declined`/`failed` entries show no badge: there is
 *  nothing new for the candidate to notice on the card itself. */
export function autoApplyNeedsReviewBadge(status?: string | null): boolean {
  return status === 'pending_review' || status === 'blocked';
}

export type AutoApplyReviewBanner =
  | { kind: 'pending_review' }
  | { kind: 'blocked' }
  | { kind: 'declined' }
  | { kind: 'failed' }
  | null;

/** Decides which drawer banner variant to render, or null for `tailoring`/`approved` —
 *  states with nothing yet for the candidate to see or decide. `pending_review` is the one
 *  actionable variant (approve/decline); `blocked`/`declined`/`failed` are read-only, and
 *  the drawer's own copy for them must never imply a retry is possible — no retry path
 *  exists anywhere in the backend for any of the three. */
export function autoApplyReviewBanner(status?: string | null): AutoApplyReviewBanner {
  switch (status) {
    case 'pending_review':
      return { kind: 'pending_review' };
    case 'blocked':
      return { kind: 'blocked' };
    case 'declined':
      return { kind: 'declined' };
    case 'failed':
      return { kind: 'failed' };
    default:
      return null;
  }
}
