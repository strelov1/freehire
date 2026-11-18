import { api } from '#lib/api.js';
import { fromApi } from '#lib/apiSuggestions.js';
import { namedCompanies } from '#lib/dropdownRows.js';
import type { Suggestion } from '#lib/suggestions.js';
import type { ApiSuggestionPart, CompanyListItem, Job } from '#lib/types.js';

/** How much HeaderSearch asks for per section, and the shortest query worth asking about. */
export interface TypedSuggestionLimits {
  completions: number;
  jobs: number;
  companies: number;
  /** Asked for, before the relevance filter takes its cut. */
  companiesFetch: number;
  minChars: number;
}

/** What HeaderSearch's typed box offers: completions from the suggestions endpoint, and
 *  the postings and companies the text matches. Refetched for each settled `query` while
 *  `enabled`; constructed during component initialisation, since it registers an effect. */
export class TypedSuggestions {
  readonly #query: () => string;
  readonly #enabled: () => boolean;
  readonly #limits: TypedSuggestionLimits;

  completions = $state.raw<Suggestion[]>([]);
  // The parts each completion applies, by row key — kept beside the rows rather than
  // inside them because a Suggestion is what the dropdown RENDERS, and these are what
  // choosing it DOES.
  parts = $state.raw<Record<string, ApiSuggestionPart[]>>({});
  // Postings and companies for the typed text, fetched exactly the way the launcher
  // dropdown fetches them — same endpoints, same abandonment rule, same
  // row rendering below. A second implementation of "show me matching jobs" is how the
  // two would drift.
  //
  // These matter MORE now than before, not less: the list below no longer narrows as
  // you type, so these rows are the only live evidence the query finds anything.
  jobs = $state.raw<Job[]>([]);
  companies = $state.raw<CompanyListItem[]>([]);

  constructor(query: () => string, enabled: () => boolean, limits: TypedSuggestionLimits) {
    this.#query = query;
    this.#enabled = enabled;
    this.#limits = limits;

    $effect(() => {
      const typed = this.#query().trim();
      if (typed.length < this.#limits.minChars || !this.#enabled()) {
        this.completions = [];
        this.parts = {};
        this.jobs = [];
        this.companies = [];
        return;
      }
      // One controller per settled query, aborted by this effect's own cleanup. It
      // replaced a stale-response counter, which dropped the ANSWER after the request had
      // already gone out and the server had already worked for it. The signal doubles as
      // the staleness check below: abandoned and stale are the same condition here, so a
      // counter beside it would be a second answer to one question.
      const ac = new AbortController();
      void (async () => {
        // allSettled, not all: the three sections are independent, so one endpoint
        // failing still shows the sections that succeeded instead of blanking all of
        // them. The completions in particular sit behind a dictionary that is rebuilt on
        // a schedule — a cold or missing one must cost the box its completions, not its
        // postings.
        const [s, j, c] = await Promise.allSettled([
          api.suggest(typed, this.#limits.completions, ac.signal),
          // q_prefix: the draft's last word may be half-typed ("data eng").
          // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient: built and handed to the request once; not reactive state
          api.searchJobs(new URLSearchParams({ q: typed, q_prefix: 'true' }), this.#limits.jobs, 0, ac.signal),
          // Over-fetch: most of what the fuzzy endpoint returns is discarded below, and
          // asking for exactly three would leave the section empty whenever the fourth
          // was the only real match.
          api.listCompanies(typed, this.#limits.companiesFetch, 0, undefined, ac.signal),
        ]);
        // An abort rejects all three, and `allSettled` would otherwise report that as
        // three empty sections — blanking the panel a fresher query is about to fill.
        if (ac.signal.aborted) return;
        const rows = s.status === 'fulfilled' ? s.value : [];
        this.completions = fromApi(rows);
        this.parts = Object.fromEntries(this.completions.map((row, i) => [row.slug, rows[i]?.parts ?? []]));
        this.jobs = j.status === 'fulfilled' ? j.value.items : [];
        this.companies = c.status === 'fulfilled' ? namedCompanies(c.value.items, typed, this.#limits.companies) : [];
      })();
      return () => ac.abort();
    });
  }
}
