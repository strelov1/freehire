import { SvelteSet } from 'svelte/reactivity';
import { track } from '../../lib/analytics';
import { getToken } from '../../lib/auth';
import { isFreehireOrigin, markApplied, type FreehireJob } from '../../lib/freehire';
import { leftAnApplication } from '../../lib/applyPlan';
import type { FormPlan } from './formPlan.svelte';
import type { PageMatch } from './pageMatch.svelte';

/** The tab the panel last read: its id and URL. */
export interface CurrentTab {
  id: number | null;
  url: string;
}

/**
 * Follows a started application to its end, for the "did you submit?" prompt and for
 * LinkedIn's own "application was sent" screen, which records the job without asking.
 */
export class AppliedTracker {
  /** The catalog job whose started application gave way to a page with no form — the one
   *  the "did you submit?" prompt names. Kept until answered. */
  candidate = $state<FreehireJob | null>(null);
  /** The catalog job whose application form is in front of the user, started. */
  private startedJob: FreehireJob | null = null;
  /** The tab the started application lives in, and the tab the prompt belongs to: on another
   *  tab (the tailor page, another job) it stays quiet. */
  private startedTabId: number | null = null;
  private appliedTabId: number | null = null;
  /** Jobs already recorded from LinkedIn's own confirmation, so a re-read does not post twice. */
  private readonly autoApplied = new SvelteSet<string>();

  constructor(
    private readonly form: FormPlan,
    private readonly pageMatch: PageMatch,
    private readonly tab: () => CurrentTab,
    private readonly notify: (text: string) => void,
  ) {}

  /** Asked only once the page in front of the user shows no form: the next step of a
   *  multi-step application is still the application, not its end. */
  asks(autofilling: boolean): boolean {
    const { candidate, form, pageMatch } = this;
    const tab = this.tab();
    return (
      candidate !== null &&
      !form.plan &&
      !autofilling &&
      tab.id === this.appliedTabId &&
      !isFreehireOrigin(tab.url) &&
      (pageMatch.status !== 'ready' || pageMatch.job?.public_slug === candidate.public_slug)
    );
  }

  /**
   * Follows the application through form reads rather than page events: a full navigation
   * and a single-page ATS replacing its form with a thank-you both end the same way — a
   * read that finds no form after one that found a started application. Page events
   * cannot see the second, and race the page's own change notice on the first.
   */
  note() {
    const { form, pageMatch } = this;
    if (form.sent) {
      void this.recordSent();
      return;
    }
    // An Easy Apply dialog, prefilled, already looks started and its closing proves nothing:
    // LinkedIn's own "application was sent" screen (above) is the signal there.
    if (form.easyApply) return;
    if (leftAnApplication(form.plan)) {
      if (pageMatch.job?.public_slug) {
        this.startedJob = pageMatch.job;
        this.startedTabId = this.tab().id;
      }
      return;
    }
    if (!form.plan && this.startedJob) {
      this.candidate = this.startedJob;
      this.appliedTabId = this.startedTabId;
      this.startedJob = null;
    }
  }

  /** LinkedIn said the application was sent: record it for the job this tab shows. */
  private async recordSent() {
    const job = this.pageMatch.status === 'ready' ? this.pageMatch.job : null;
    const token = await getToken();
    if (!job?.public_slug || !token || this.autoApplied.has(job.public_slug)) return;
    this.autoApplied.add(job.public_slug);
    try {
      await markApplied(job.public_slug, token);
      track('mark_applied', { source: 'linkedin_confirmation' });
      this.notify(`✓ LinkedIn confirmed it — ${job.title} is marked as applied.`);
      // Only this job's tracking: another application may have started meanwhile.
      if (this.startedJob?.public_slug === job.public_slug) this.startedJob = null;
      if (this.candidate?.public_slug === job.public_slug) this.candidate = null;
    } catch (err) {
      this.autoApplied.delete(job.public_slug);
      this.notify(`Could not mark as applied: ${err instanceof Error ? err.message : 'error'}`);
    }
  }
}
