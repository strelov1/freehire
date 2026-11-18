import { untrack } from 'svelte';
import { goto } from '$app/navigation';
import { page } from '$app/state';
import { api } from '#lib/api.js';
import { hasCompanyDetails } from '#lib/companyDetails.js';
import type { Display } from '#lib/generated/contracts.js';
import type { Company, Job } from '#lib/types.js';
import { applyFormWorthShowing } from './JobApplyForm.svelte';

export type ContentTab = 'description' | 'company' | 'application' | 'discussion';

// The content column is tabbed so "who are these people?" and "what will they ask
// me?" are answerable without leaving the posting. Page state, not a route: the
// company copy already has a canonical home at /companies/<slug>, and a second URL
// serving it would be a thin duplicate we'd then have to keep out of the index.
//
// Built per job rather than fixed, because neither Company nor Application is always
// there: we may know nothing about the employer, and only a few ATS platforms publish a
// form we can read. A tab is offered only when its panel has something in it —
// `applyFormWorthShowing` is the same predicate the panel itself renders on, so the two
// cannot disagree about whether there is anything to show. Discussion is always there.
//
// JobView's content tabs: which exist for this job, the one shown, the company behind the
// Company tab and the open-thread count on Discussion. Constructed during component
// initialisation (it registers effects) with getters for the job, its closure and its
// application form.
export class JobContentTabs {
  readonly #job: () => Job;
  readonly #closed: () => boolean;
  readonly #applyForm: () => Display | null;

  // Open-thread count for the "Discussion · N" badge; loaded client-side so the
  // page renders immediately and the number fills in. Failures leave it hidden.
  threadCount = $state<number | null>(null);

  readonly companySlug = $derived.by(() => this.#job().company_slug ?? '');
  // The company behind the Company tab, read in the browser only (see JobCompanyPanel for
  // why it is not server-rendered). The tab appears once the read shows there is
  // something to put in it: an empty "no details yet" tab was a click to nowhere. A failed
  // read leaves the tab out too; the company row's link still reaches the company page.
  company = $state.raw<Company | null>(null);
  readonly companyWithDetails = $derived(this.company && hasCompanyDetails(this.company) ? this.company : null);
  readonly tabs: { id: ContentTab; label: string }[] = $derived.by(() => [
    { id: 'description', label: 'Description' },
    ...(this.companyWithDetails ? [{ id: 'company' as const, label: 'Company' }] : []),
    // The screening form is pointless once the posting stops taking applications.
    ...(!this.#closed() && applyFormWorthShowing(this.#applyForm())
      ? [{ id: 'application' as const, label: 'Application' }]
      : []),
    { id: 'discussion', label: this.threadCount ? `Discussion · ${this.threadCount}` : 'Discussion' },
  ]);
  // `?tab=discussion` is where the old /jobs/<slug>/discussion page redirects, so its
  // links (a thread's "Back to discussion") land on the tab they meant.
  #initialTab = (): ContentTab => (page.url.searchParams.get('tab') === 'discussion' ? 'discussion' : 'description');
  current = $state<ContentTab>(this.#initialTab());
  // Discussion is mirrored into the URL so Back from a topic, and the return from sign-in,
  // land on the tab again. The other tabs leave the URL clean.
  select = (id: ContentTab) => {
    this.current = id;
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient: built, edited and handed to goto once; not reactive state
    const url = new URL(page.url.href);
    if (id === 'discussion') url.searchParams.set('tab', 'discussion');
    else url.searchParams.delete('tab');
    // A shallow replace: the address changes, nothing reloads and Back is not doubled.
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- the current page's own URL, only its query changed
    if (url.href !== page.url.href) void goto(url, { replace: true, shallow: true, state: page.state });
  };

  constructor(job: () => Job, closed: () => boolean, applyForm: () => Display | null) {
    this.#job = job;
    this.#closed = closed;
    this.#applyForm = applyForm;

    $effect(() => {
      let alive = true;
      // Cleared first: on a client-side move to another job the tab would keep the last
      // job's count until the new one arrives.
      this.threadCount = null;
      api
        .countThreads('job', this.#job().public_slug)
        .then((n) => alive && (this.threadCount = n))
        .catch(() => {});
      return () => {
        alive = false;
      };
    });

    $effect(() => {
      const slug = this.companySlug;
      this.company = null;
      if (!slug) return;
      let stale = false;
      // `limit=1` is the endpoint's floor; the one job that comes back is discarded.
      api
        .getCompany(slug, 1, 0)
        .then((r) => {
          if (!stale) this.company = r.company;
        })
        .catch(() => {});
      return () => {
        stale = true;
      };
    });

    // Reset when the visitor navigates client-side to another job: JobView is not
    // remounted on a param change, so the Company tab would otherwise stay selected
    // over a company they never asked about.
    $effect(() => {
      void this.#job().public_slug;
      this.current = untrack(this.#initialTab);
    });
  }
}
