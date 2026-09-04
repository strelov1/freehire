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
  | { kind: 'declined' }; // the candidate's own prior decision, permanent

/** Decides the auto-apply button's state from the job's source and the caller's own
 *  auto_apply_status (undefined/null for no attempt or an anonymous caller). */
export function autoApplyButtonState(source: string, status?: string | null): AutoApplyButtonState {
  if (source !== 'greenhouse') return { kind: 'hidden' };
  if (status === 'queued') return { kind: 'queued' };
  if (status === 'declined') return { kind: 'declined' };
  return { kind: 'idle' };
}
