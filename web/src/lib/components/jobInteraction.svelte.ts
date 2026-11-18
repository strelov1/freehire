import { api } from '#lib/api.js';
import { track } from '#lib/analytics.js';
import { currentUser } from '#lib/auth.svelte.js';
import { markAppliedToCompany } from '#lib/appliedCompanies.svelte.js';
import { markSaved, markUnsaved } from '#lib/savedJobs.svelte.js';
import type { Job, UserJob } from '#lib/types.js';
import { markViewed } from '#lib/viewedJobs.svelte.js';

/**
 * The signed-in user's interaction with the job page's posting: the view it records, the
 * save toggle, and the "Did you apply?" question the Apply link raises. Every answer is the
 * server's, so the page can never drift from what tracking holds.
 */
export class JobInteraction {
  /** Null when signed out or not yet loaded. */
  current = $state.raw<UserJob | null>(null);
  /** The post-click "Did you apply?" question is showing. */
  askApplied = $state(false);
  /** Set after the user confirms "Yes" on the apply prompt: surfaces a one-tap link to the
   *  Tracking board where the job now sits. Reset when the job changes. */
  justApplied = $state(false);

  constructor(private readonly job: () => Job) {}

  /** A different posting: forget everything said about the last one. */
  reset() {
    this.current = null;
    this.askApplied = false;
    this.justApplied = false;
  }

  /** Silent history that also tells us whether they already applied; a failed view must
   *  not break the page. */
  record(slug: string) {
    api
      .recordJobView(slug)
      .then((rec) => {
        // Mark it locally so its card dims on back-navigation without a reload.
        markViewed(slug);
        if (this.job().public_slug === slug) this.current = rec;
      })
      .catch(() => {});
  }

  confirmApplied = async () => {
    const { public_slug: slug, company_slug: company } = this.job();
    const userID = currentUser()?.id;
    let result: UserJob;
    try {
      result = await api.markJobApplied(slug);
    } catch {
      // Leave the prompt up so the user can retry; nothing else to do.
      return;
    }
    if (currentUser()?.id !== userID) return;
    markAppliedToCompany(company);
    track('job_track', { slug, stage: 'applied' });
    if (this.job().public_slug !== slug) return;
    this.current = result;
    this.askApplied = false;
    this.justApplied = true;
  };

  /** "No": purely local — the job must not enter the tracker. */
  dismiss = () => {
    this.askApplied = false;
  };

  /**
   * What the direct-apply dialog calls once the application is sent. Built from the posting
   * on screen before the dialog starts its asynchronous send, so a reply landing after the
   * reader moved on (or signed in as someone else) never marks the wrong page.
   * `onDelivered` runs only while that posting is still the one shown.
   */
  sentCallback(onDelivered: () => void) {
    const { public_slug: slug, company_slug: company } = this.job();
    const userID = currentUser()?.id;
    return () => {
      if (currentUser()?.id !== userID) return;
      markAppliedToCompany(company);
      track('job_track', { slug, stage: 'applied' });
      if (this.job().public_slug !== slug) return;
      onDelivered();
      if (this.current) {
        // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient: only its ISO string is kept
        const now = new Date().toISOString();
        this.current = { ...this.current, applied_at: this.current.applied_at ?? now };
      }
      this.justApplied = true;
    };
  }

  /** The toggle flips on the server's answer, not optimistically: both endpoints return the
   *  full interaction, so the button can never drift from the truth. */
  toggleSave = async () => {
    const slug = this.job().public_slug;
    try {
      this.current = this.current?.saved_at != null ? await api.unsaveJob(slug) : await api.saveJob(slug);
      // Keep the shared saved set in sync so a browse card's bookmark reflects this on
      // back-navigation without a reload (mirrors markViewed above).
      if (this.current?.saved_at != null) {
        markSaved(slug);
        track('job_save', { slug });
      } else markUnsaved(slug);
    } catch {
      // Leave the current state; the user can retry.
    }
  };
}
