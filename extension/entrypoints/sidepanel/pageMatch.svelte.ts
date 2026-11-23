import { pageIdentity } from '../../lib/assistant/pageIdentity';
import { browser } from 'wxt/browser';
import { getToken } from '../../lib/auth';
import { type FreehireJob, type JobMatch } from '../../lib/freehire';
import { contributePageUrl, matchPage } from '../../lib/pageMatch';

export type MatchStatus = 'idle' | 'loading' | 'ready' | 'error' | 'empty';

/**
 * The "Match" tab's account of the active page: the freehire job it is, the caller's
 * match against it, and the "Add this job" offer when it is not one. `load` re-reads it
 * on every tab switch and page load; `contribute` hands the page to freehire.
 */
export class PageMatch {
  status = $state<MatchStatus>('idle');
  job = $state<FreehireJob | null>(null);
  match = $state<JobMatch | null>(null);
  error = $state('');
  // Whether the empty state offers "Add this job": only on a page that is plausibly a
  // vacancy (lib/addJob.ts), decided alongside the miss that put the panel there.
  canAddJob = $state(false);
  contributing = $state(false);
  /** While true (an Easy Apply dialog is open), a re-read keeps the card on screen instead of
   *  swapping it for the loading skeleton — LinkedIn fires page events as the dialog opens. */
  keepCard = false;

  // Bumped at the start of every load() call; a stale call checks its own
  // captured id against the current one before writing shared state, so an
  // earlier in-flight load (e.g. a slow ad-hoc text match) can never clobber a
  // newer one's result after the user has already tabbed or navigated again.
  #requestId = 0;
  /** The page the ready card belongs to: only that page may keep it through a reload. */
  #cardPage = '';

  /** `notify` adds a line to the panel's action notices. */
  constructor(private readonly notify: (notice: string) => void) {}

  /** Forgets the card: on sign-out it belongs to the account that left, and an in-flight
   *  load for that account must not write it back. */
  reset() {
    this.#requestId++;
    this.#cardPage = '';
    this.status = 'idle';
    this.job = null;
    this.match = null;
  }

  load = async () => {
    const requestId = ++this.#requestId;
    const token = await getToken();
    if (requestId !== this.#requestId) return;
    if (!token) {
      this.status = 'empty';
      this.canAddJob = false;
      this.job = null;
      this.match = null;
      return;
    }
    const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
    if (requestId !== this.#requestId) return;
    const url = tab?.url ?? '';

    const page = cardPage(url);
    if (!(this.keepCard && this.status === 'ready' && this.#cardPage === page)) this.status = 'loading';
    this.error = '';
    try {
      const result = await matchPage(url, token, () => requestId !== this.#requestId);
      if (!result) return;
      if (result.status === 'ready') {
        this.job = result.job;
        this.match = result.match;
        this.#cardPage = page;
      } else {
        this.canAddJob = result.canAddJob;
      }
      this.status = result.status;
    } catch (err) {
      if (requestId !== this.#requestId) return;
      this.error = err instanceof Error ? err.message : 'Could not load match';
      this.status = 'error';
    }
  };

  /**
   * Hands the current page to freehire. The server imports the vacancy when a link-source
   * adapter can read the page and queues the link for a maintainer when none can; either
   * way the panel says what happened, and a resolved slug re-runs the match so the curated
   * card replaces the ad-hoc one.
   */
  contribute = async () => {
    // Claimed before the first await: `disabled` only follows the flag, so a second
    // activation landing while the token is being read would otherwise pass the guard
    // too and post the page twice — two imports, and two charges against the rate limit
    // the endpoint carries.
    if (this.contributing) return;
    this.contributing = true;
    try {
      const token = await getToken();
      if (!token) return;
      const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
      const url = tab?.url ?? '';
      if (!url) {
        this.notify('No page to add.');
        return;
      }
      const resolved = await contributePageUrl(url, token);
      this.notify(resolved.notice);
      if (resolved.slug) await this.load();
    } catch (err) {
      this.notify(`Could not add this page: ${err instanceof Error ? err.message : 'error'}`);
    } finally {
      this.contributing = false;
    }
  };
}

/** The page a card belongs to. LinkedIn's search view picks the job by `currentJobId`, which
 *  `pageIdentity` drops, so a dialog open on job A must not keep A's card on job B. */
function cardPage(url: string): string {
  const jobId = URL.canParse(url) ? new URL(url).searchParams.get('currentJobId') : null;
  return jobId ? `${pageIdentity(url)}?currentJobId=${jobId}` : pageIdentity(url);
}
