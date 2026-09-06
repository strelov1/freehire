// The auto-apply button's rendered state, decided from the job detail response's own
// fields plus the caller's plan — kept out of JobView.svelte so it unit-tests without
// mounting Svelte, mirroring notificationTarget.ts's own convention.
//
// A non-Pro caller sees nothing at all, not an idle-but-clickable button: this is a Pro
// feature end to end, so the gate is visibility, not just what happens on click. Base-CV
// existence is still NOT decided here — that one caller learns about from the backend's
// own 409 after clicking (JobView.svelte's error handling, not this function), since
// whether a base CV exists is not plan-shaped and isn't worth a second client-side fetch
// just to pre-empt one specific 409.

// The ATS providers auto-apply can queue an attempt against at all — kept as a Set (not a
// backend-fetched list) because the frontend only needs to decide hidden-vs-not; whether a
// given attempt can actually SUBMIT (Greenhouse today) is the backend's own concern, not
// this button's. Mirrors autoApplyEnqueueSources in internal/api/handler/auto_apply_enqueue.go.
const autoApplyProviders = new Set(['greenhouse', 'ashby', 'workable', 'lever', 'recruitee']);

export type AutoApplyButtonState =
  | { kind: 'hidden' } // not Pro, or an ATS auto-apply cannot queue an attempt against at all
  | { kind: 'idle' } // no attempt yet — the button is clickable
  | { kind: 'queued' } // a live, undecided attempt already exists
  | { kind: 'declined' } // the candidate's own prior decision, permanent
  | { kind: 'applied' } // a completed auto-apply already submitted this job for real
  | { kind: 'failed' }; // cmd/auto-apply gave up on this attempt (dead-lettered or parked)

/** Decides the auto-apply button's state from the job's source, the caller's own
 *  auto_apply_status (undefined/null for no attempt or an anonymous caller), whether they
 *  already applied, and whether they are on the Pro plan (or above). `isPro` is checked
 *  first and wins over everything else, including a standing attempt from before a lapsed
 *  subscription: a non-Pro caller sees no auto-apply state for this job at all, not a
 *  frozen queued/failed/declined badge they can no longer act on.
 *  `alreadyApplied` wins over `status`: a completed submission deletes the queue row that
 *  `status` reads (cmd/auto-apply/store.go's Submit), so the two never disagree in
 *  practice, but checking `alreadyApplied` first is what stops a re-click from starting a
 *  genuine second ATS submission if they ever did. */
export function autoApplyButtonState(
  source: string,
  status: string | null | undefined,
  alreadyApplied: boolean,
  isPro: boolean
): AutoApplyButtonState {
  if (!isPro) return { kind: 'hidden' };
  if (!autoApplyProviders.has(source)) return { kind: 'hidden' };
  if (alreadyApplied) return { kind: 'applied' };
  if (status === 'queued') return { kind: 'queued' };
  if (status === 'declined') return { kind: 'declined' };
  if (status === 'failed') return { kind: 'failed' };
  return { kind: 'idle' };
}

/** What the job page's two call-to-action buttons look like, given the auto-apply state.
 *  Never two loud buttons at once, and one wherever the reader still has something to do.
 *  Carries no Pro marker: `autoApplyButtonState` already gates the whole button on Pro, so
 *  by the time any of these states renders at all the requirement is already met — a badge
 *  naming it here would state a fact about nothing left to decide. */
export type JobCtaPlan = {
  /** `null` where auto-apply cannot drive the posting's ATS, or the caller is not Pro: no
   *  button is rendered either way — see autoApplyButtonState's own `hidden` doc comment. */
  autoApply: {
    label: string;
    /** Carries the brand fill — the page's primary call to action. */
    primary: boolean;
    disabled: boolean;
  } | null;
  /** The link out to the posting's own site. Demoted to an outline `Show origin` while
   *  auto-apply owns the primary slot. */
  external: { label: 'Apply' | 'Show origin'; primary: boolean };
};

/** A rendered-but-unpressable auto-apply button: it reports where the attempt stands and
 *  takes no brand fill. */
const quiet = (label: string): NonNullable<JobCtaPlan['autoApply']> => ({
  label,
  primary: false,
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
        autoApply: { label: 'Auto-apply', primary: true, disabled: false },
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
