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
  | { kind: 'tailoring' }
  | { kind: 'pending_review' }
  | { kind: 'approved' }
  | { kind: 'blocked' }
  | { kind: 'declined' }
  | { kind: 'failed' }
  | null;

/** Decides which drawer banner variant to render, or null when there is no live attempt at
 *  all. `pending_review` is the one actionable variant (approve/decline); `tailoring` and
 *  `approved` are read-only progress indicators — nothing to decide yet, or the decision is
 *  already made — and `blocked`/`declined`/`failed` are read-only terminal states, whose
 *  drawer copy must never imply a retry is possible — no retry path exists anywhere in the
 *  backend for any of the three. */
export function autoApplyReviewBanner(status?: string | null): AutoApplyReviewBanner {
  switch (status) {
    case 'tailoring':
      return { kind: 'tailoring' };
    case 'pending_review':
      return { kind: 'pending_review' };
    case 'approved':
      return { kind: 'approved' };
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
