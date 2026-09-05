// The auto-apply button's rendered state, decided from the two fields the job detail
// response already carries — kept out of JobView.svelte so it unit-tests without mounting
// Svelte, mirroring notificationTarget.ts's own convention.
//
// PRO-eligibility and base-CV existence are NOT decided here: neither is known
// client-side without new plumbing this change does not add (see
// openspec/changes/auto-apply-submit-trigger/design.md's own Goals). A caller who is
// not eligible still sees the idle button and learns why from the backend's own 402/409
// message after clicking — JobView.svelte's error handling, not this function.

export type AutoApplyButtonState =
  | { kind: 'hidden' } // not a Greenhouse posting — auto-apply cannot resolve this ATS yet
  | { kind: 'idle' } // no attempt yet — the button is clickable
  | { kind: 'queued' } // a live, undecided attempt already exists
  | { kind: 'declined' } // the candidate's own prior decision, permanent
  | { kind: 'applied' } // a completed auto-apply already submitted this job for real
  | { kind: 'failed' }; // cmd/auto-apply gave up on this attempt (dead-lettered or parked)

/** Decides the auto-apply button's state from the job's source, the caller's own
 *  auto_apply_status (undefined/null for no attempt or an anonymous caller), and whether
 *  they already applied. `alreadyApplied` wins over `status`: a completed submission
 *  deletes the queue row that `status` reads (cmd/auto-apply/store.go's Submit), so the two
 *  never disagree in practice, but checking `alreadyApplied` first is what stops a re-click
 *  from starting a genuine second ATS submission if they ever did. */
export function autoApplyButtonState(
  source: string,
  status?: string | null,
  alreadyApplied?: boolean
): AutoApplyButtonState {
  if (source !== 'greenhouse') return { kind: 'hidden' };
  if (alreadyApplied) return { kind: 'applied' };
  if (status === 'queued') return { kind: 'queued' };
  if (status === 'declined') return { kind: 'declined' };
  if (status === 'failed') return { kind: 'failed' };
  return { kind: 'idle' };
}
