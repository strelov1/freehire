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

/** What the job page's two call-to-action buttons look like, given the auto-apply state.
 *  Never two loud buttons at once, and one wherever the reader still has something to do. */
export type JobCtaPlan = {
  /** `null` where auto-apply cannot drive the posting's ATS: no button is rendered. */
  autoApply: {
    label: string;
    /** Carries the brand fill — the page's primary call to action. */
    primary: boolean;
    /** Renders the `Pro` marker naming the plan the action requires. */
    pro: boolean;
    disabled: boolean;
  } | null;
  /** The link out to the posting's own site. Demoted to an outline `Show origin` while
   *  auto-apply owns the primary slot. */
  external: { label: 'Apply' | 'Show origin'; primary: boolean };
};

/** A rendered-but-unpressable auto-apply button: it reports where the attempt stands and
 *  takes neither the brand fill nor the `Pro` marker. */
const quiet = (label: string): NonNullable<JobCtaPlan['autoApply']> => ({
  label,
  primary: false,
  pro: false,
  disabled: true,
});

const showOrigin = { label: 'Show origin', primary: false } as const;
const apply = { label: 'Apply', primary: true } as const;

/** Ranks the two CTAs for a posting.
 *
 *  `declined` and `failed` hand the primary slot BACK to the external button: auto-apply
 *  is not going to act in either state, so applying by hand is the reader's only way
 *  forward and demoting it there would leave the page with nothing loud to press. The rule
 *  is "demote while an attempt stands or can be started", not "demote whenever the
 *  auto-apply button exists" — the two read the same until you reach those two states.
 *
 *  `pro` rides only the clickable state for the same reason the brand fill does: a marker
 *  naming what an action requires says nothing on a button nobody can press.
 *
 *  `applied` does NOT demote, even though the reader has nothing left to do here. That
 *  state comes from `alreadyApplied` — the "Did you apply?" prompt after a manual
 *  click-through — and is true of a posting from any source, while this table only runs on
 *  the ones auto-apply can drive. Demoting on it would make a Greenhouse posting read
 *  differently from an identical Lever one for a reader in the identical situation, which
 *  is an artefact of routing the question through the auto-apply state machine rather than
 *  a decision anybody made. `queued` is the one state that genuinely leaves no primary CTA,
 *  and it is genuinely auto-apply's own. */
export function jobCtaPlan(state: AutoApplyButtonState): JobCtaPlan {
  switch (state.kind) {
    case 'hidden':
      return { autoApply: null, external: apply };
    case 'idle':
      return {
        autoApply: { label: 'Auto-apply', primary: true, pro: true, disabled: false },
        external: showOrigin,
      };
    case 'queued':
      return { autoApply: quiet('Auto-apply queued'), external: showOrigin };
    case 'applied':
      return { autoApply: quiet('Already applied'), external: apply };
    case 'declined':
      return { autoApply: quiet('Auto-apply declined'), external: apply };
    case 'failed':
      return { autoApply: quiet("Auto-apply couldn't complete"), external: apply };
  }
}
