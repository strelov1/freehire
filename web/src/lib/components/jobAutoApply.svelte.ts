import { ApiError, api } from '#lib/api.js';
import { track } from '#lib/analytics.js';
import { isAuthenticated } from '#lib/auth.svelte.js';
import type { Job, PlanState } from '#lib/types.js';

/**
 * The job page's auto-apply request: queuing the posting and what the button shows until
 * the page reloads. Base-CV existence is the one eligibility fact still not known
 * client-side — the backend's own 409 is what tells a caller with no base CV why, surfaced
 * as `error` rather than pre-empted here.
 */
export class JobAutoApply {
  /** The status this page set ('queued' once a request succeeds), over the job's own. */
  overrideStatus = $state<string | null>(null);
  submitting = $state(false);
  error = $state<string | null>(null);

  /** The caller's own plan (null when signed out or not yet loaded): auto-apply is the
   *  one caller-scoped feature on the job page whose visibility, not just its outcome,
   *  depends on the plan tier. */
  plan = $state.raw<PlanState | null>(null);
  isPro = $derived(this.plan?.plan === 'pro' || this.plan?.plan === 'ultra');

  /** Construct during component initialisation: it fetches the plan in an effect. */
  constructor(private readonly job: () => Job) {
    // Fetched once per signed-in session rather than per job: it does not depend on the
    // posting, so navigating between job pages does not re-fetch or flicker it back to
    // null while a plan that has not changed reloads. A failed fetch leaves it null, which
    // the auto-apply gate treats the same as "not Pro" — a safe, if conservative, default
    // rather than a broken page.
    $effect(() => {
      if (!isAuthenticated()) {
        this.plan = null;
        return;
      }
      let alive = true;
      api
        .myPlan()
        .then((p) => alive && (this.plan = p))
        .catch(() => {});
      return () => {
        alive = false;
      };
    });
  }

  start = async () => {
    const { public_slug: slug } = this.job();
    this.error = null;
    this.submitting = true;
    try {
      await api.autoApplyJob(slug);
      this.overrideStatus = 'queued';
      track('job_auto_apply', { slug });
    } catch (err) {
      this.error = err instanceof ApiError ? err.message : 'Something went wrong — please try again.';
    } finally {
      this.submitting = false;
    }
  };
}
