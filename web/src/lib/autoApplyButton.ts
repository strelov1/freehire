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

/** What the job page's two APPLY controls look like, given the auto-apply state.
 *
 *  This plan ranks nothing by colour. The job page's one primary (brand-filled) call to
 *  action is `Tailor my CV`, in every state — see JobView.svelte's `tailorCta` — so both
 *  controls here are quiet ones, and what is left to decide is which of them is the offered
 *  way to APPLY. That question is not cosmetic: it is what puts one of them in the phone's
 *  sticky bar beside the tailoring button, and what the external link's own word reports. */
export type JobCtaPlan = {
  /** `null` where auto-apply cannot drive the posting's ATS, or the caller is not Pro: no
   *  button is rendered either way — see autoApplyButtonState's own `hidden` doc comment. */
  autoApply: {
    label: string;
    /** Auto-apply is the posting's offered way to apply: it takes the phone's sticky bar,
     *  and the external link steps aside into the quiet strip under the title. */
    leads: boolean;
    /** Renders the `Pro` marker naming the plan the action requires. */
    pro: boolean;
    disabled: boolean;
  } | null;
  /** The link out to the posting's own site, an outline button in every state. Reads
   *  `Show origin` while auto-apply is doing the applying — the word is the demotion. */
  external: { label: 'Apply' | 'Show origin' };
};

/** A rendered-but-unpressable auto-apply button: it reports where the attempt stands, leads
 *  nothing and takes no `Pro` marker. */
const quiet = (label: string): NonNullable<JobCtaPlan['autoApply']> => ({
  label,
  leads: false,
  pro: false,
  disabled: true,
});

const showOrigin = { label: 'Show origin' } as const;
const apply = { label: 'Apply' } as const;

/** Ranks the two apply controls for a posting.
 *
 *  `declined` and `failed` give the external link its own word BACK: auto-apply is not going
 *  to act in either state, so applying by hand is the reader's only way forward and calling
 *  that link `Show origin` would name it as the second-best route to a door that is now the
 *  only one. The rule is "relabel while an attempt stands or can be started", not "relabel
 *  whenever the auto-apply button exists" — the two read the same until you reach those two
 *  states.
 *
 *  `applied` does NOT relabel, even though the reader has nothing left to do here. That
 *  state comes from `alreadyApplied` — the "Did you apply?" prompt after a manual
 *  click-through — and is true of a posting from any source, while this table only runs on
 *  the ones auto-apply can drive. Relabelling on it would make a Greenhouse posting read
 *  differently from an identical Lever one for a reader in the identical situation, which
 *  is an artefact of routing the question through the auto-apply state machine rather than
 *  a decision anybody made.
 *
 *  `queued` relabels without leading: the attempt is in flight, so the link is genuinely the
 *  second route, but nothing about it should invite a second submission — which is why the
 *  phone's bar takes the link rather than a disabled auto-apply button.
 *
 *  `pro` rides only the clickable state: a marker naming what an action requires says
 *  nothing on a button nobody can press. */
export function jobCtaPlan(state: AutoApplyButtonState): JobCtaPlan {
  switch (state.kind) {
    case 'hidden':
      return { autoApply: null, external: apply };
    case 'idle':
      return {
        autoApply: { label: 'Auto-apply', leads: true, pro: true, disabled: false },
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
