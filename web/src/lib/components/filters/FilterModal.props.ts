import type { Snippet } from 'svelte';
import type { FilterStore, JobFilters } from '#lib/filters.js';
import type { StagedFilters } from '#lib/stagedFilters.svelte.js';
import type { FacetCounts } from '#lib/types.js';

/**
 * The job-search filter modal: a thin wrapper over FilterModalShell that supplies the
 * job rail, the staged job filters, and the pane controls. The shell owns the chrome
 * (rail, footer, deferred apply); FilterModal.svelte owns only what's job-specific.
 *
 * Reusable beyond the standalone list: `railKeys` restricts which rail panes show
 * (e.g. Specialization + Skills for a profile); `applyLabel`/`onApply` give the footer
 * a custom label and action (e.g. "Save" → persist a profile); `canApply` gates it.
 */
export type FilterModalProps = {
  store?: FilterStore;
  seed?: JobFilters;
  counts?: FacetCounts | null;
  exclude?: string[];
  railKeys?: string[];
  // Show the "My filters" (saved searches) tab. Opt-in: the standalone job list enables
  // it; reuse like the profile comparison modal leaves it off.
  savedSearches?: boolean;
  /** Personal company filter: enabled only by a feed whose search uses the session. */
  formerEmployersAvailable?: boolean;
  /** Personal watchlist filter: enabled by the same feeds as the former-employers one. */
  watchlistAvailable?: boolean;
  /** "Just started hiring" company filter: enabled by the feeds that serve /jobs/search. */
  newHiringAvailable?: boolean;
  hideSeenAvailable?: boolean;
  title?: string;
  applyLabel?: string;
  onApply?: (staged: StagedFilters) => void | Promise<void>;
  canApply?: (f: JobFilters) => boolean;
  // Plain-select reuse (e.g. the profile editor): drop the search-only exclude/match
  // toggles so a facet value reads as a plain choice, not a filter.
  plain?: boolean;
  open?: boolean;
  onClose: () => void;
  previewCount?: (params: URLSearchParams) => Promise<number>;
  // Live disjunctive facet counts for the staged selection — when supplied, every
  // control shows counts that recompute as you pick (the job list). Absent for the
  // analytics/swipe reuse, which keep the applied `counts` + total-only preview.
  stagedCounts?: (params: URLSearchParams) => Promise<FacetCounts>;
  // Extra content rendered above the pane, handed the staged store so it can edit it
  // (e.g. the profile editor's "import skills from CV").
  extra?: Snippet<[StagedFilters]>;
  // The "Minimum skill match" slider atop the Skills pane: a threshold over the
  // viewer's own profile skills (`minSkillMatch` in JobFilters, applied by the server under
  // best match, in the browser by JobsView under any other ordering). Hidden unless the caller has a real percent to filter on.
  matchAvailable?: boolean;
};
