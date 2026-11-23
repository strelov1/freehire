// The job filter state, split out of facetModel.ts, which re-exports both types.
import type { FacetSelection } from './facets';
import type { JobSort } from './jobSort';

/** One facet's selection: the included values and the excluded values (a value is
 *  in at most one set), plus whether the *included* values are ANDed (match all)
 *  instead of ORed (match any). Excluded values are always ANDed — a job matches
 *  only if it has none of them. Include and exclude coexist in one facet, so a
 *  user can include some values and exclude others at the same time.
 *
 *  Structurally identical to (and aliased from) facets.ts's `FacetSelection`, the
 *  shape `FacetSection` reads — one canonical type so the two can't drift. */
export type FacetState = FacetSelection;

export interface JobFilters {
  q: string;
  /** Facet state keyed by the facet's query param (see FACETS). */
  facets: Record<string, FacetState>;
  visa: boolean;
  /** Hide the postings of employers candidates have reported screening with an AI
   *  interviewer. A boolean, not a set of levels like `clearance_levels`: that facet
   *  answers several different people, while nobody searches FOR an AI screen — the
   *  only useful question here is "not these".
   *
   *  Serialized as `ai_interview=false`, whose value names the FACET while this names
   *  the INTENT. It has to be the negative of the positive rather than an equality,
   *  because nothing in the index is ever written false. */
  hideAIInterview: boolean;
  /** Keep only postings whose ATS provider is one auto-apply can currently attempt
   *  to fill and submit (Greenhouse, Lever, Ashby, Workable) — a best-effort
   *  provider-eligibility signal, never a guarantee a real attempt would succeed.
   *
   *  Serialized as `auto_apply_available=true`, true-or-absent like
   *  `hideAIInterview`'s underlying facet: nothing in the index is ever written
   *  false, so turning this off must omit the param rather than write `=false`. */
  autoApplyAvailable: boolean;
  /** "Hiring again": keep only postings at companies the signed-in reader has worked at
   *  and left, read from their own experience bank by the server. Serialized as
   *  `former_employers=true`, true-or-absent. It is a fact about the READER, not the
   *  posting, so it never reaches a saved search (see savedSearchQuery); signed out, the
   *  server ignores it and says so in `meta.ignored_params`. */
  formerEmployers: boolean;
  /** "Your watchlist": keep only postings at the companies on the signed-in reader's own
   *  watchlist, resolved by the server. Serialized as `watchlist=true`, true-or-absent.
   *  Unlike `formerEmployers` it IS kept in a saved search: the saved search
   *  `watchlist=true` is the alert, and the digest matcher resolves it against its
   *  owner's list. Signed out, the server ignores it and says so in `meta.ignored_params`. */
  watchlist: boolean;
  /** "Just started hiring": keep only postings at companies that had nothing open for 90
   *  days and started hiring in the last 30, resolved by the server from the company
   *  rollup. Serialized as `new_hiring=true`, true-or-absent. The same for every reader,
   *  but the digest matcher cannot resolve it, so it never reaches a saved search (see
   *  savedSearchQuery). */
  newHiring: boolean;
  /** Keep only postings whose captured application form requires at most this many
   *  questions beyond the CV and contact details (null = no bound). The filter modal's
   *  "Short application" toggle writes SHORT_APPLICATION_MAX; any other bound arrives
   *  from a shared link or the API and is kept as it came. `0` is a real bound — every
   *  read must test for null, never for falsiness. Serialized as `apply_questions_max`;
   *  a posting whose form we have not read never matches it. */
  applyQuestionsMax: number | null;
  /** Minimum skill-match percent (null = any), serialized as `min_skill_match` so a saved
   *  search and the URL carry it. The percent depends on the reader, so the search never
   *  reads it (the API reports it in `meta.ignored_params`): the job list applies it to
   *  the fetched page, and the digest matcher per subscriber. The slider's `0` is "off". */
  minSkillMatch: number | null;
  salaryMin: number | null;
  /** Keep only jobs whose stated MAXIMUM salary is at most this (null = no ceiling).
   *  Serialized as `salary_max`, which bounds a different attribute than `salaryMin`
   *  does: together they keep the postings whose whole stated range fits inside
   *  [salaryMin, salaryMax], not those that merely overlap it. A posting that states
   *  only a floor carries no maximum, so any ceiling drops it. */
  salaryMax: number | null;
  /** Freshness: keep only jobs posted within the last N days (null = any age).
   *  Serialized as `posted_within_days`; the backend turns it into a posted_ts
   *  range filter relative to request time.
   *
   *  This bounds the date the SOURCE states. Some boards restate it on every crawl,
   *  so a posting open for months can satisfy a three-day bound here — see
   *  `openWithinDays`, which is the bound that cannot be rewritten from outside. */
  postedWithinDays: number | null;
  /** Keep only jobs first seen within the last N days (null = any age). Serialized as
   *  `open_within_days`; the backend turns it into a created_ts range filter.
   *
   *  Independent of `postedWithinDays` and not a substitute for it: one asks how long
   *  the posting has been in the catalogue, the other what date the board claims. Both
   *  may be set, and they narrow the list together. */
  openWithinDays: number | null;
  /** Keep only jobs asking for at most this many years of experience (null = any).
   *  `0` is a real bound — the jobs stating no prior experience is required — so
   *  every read of this field must test for null, never for falsiness. */
  experienceYearsMax: number | null;
  /** Also keep the jobs that state no experience requirement — most of the
   *  catalogue, which a bound drops otherwise. A modifier of the bound, not a filter:
   *  written to the URL only beside one, and never counted on its own. */
  experienceIncludeUnspecified: boolean;
  /** Apply the experience bound only to the jobs that carry no seniority level, so a
   *  labelled one passes it whatever years it states. Beside the included levels and
   *  `seniorityIncludeUnspecified` this reads "one of these levels, or no level and
   *  within the bound" — the entry-level profile rule (freehire#3655). A modifier of the
   *  bound: written to the URL only beside one, and never counted on its own. */
  experienceOnlySeniorityUnspecified: boolean;
  /** Also keep the jobs that carry no seniority level — most of the catalogue, which
   *  including a level drops otherwise. A modifier of the included levels: written to
   *  the URL only beside one, and never counted on its own. */
  seniorityIncludeUnspecified: boolean;
  /** Feed ordering, or `null` for "the caller has not chosen one".
   *
   *  `relevance` is the engine's own ranking, `newest` is freshest first, `views` is
   *  most-opened first, and `match` ranks by how well a vacancy's skills overlap the
   *  signed-in caller's profile. The server degrades `match` to its default for anyone
   *  it cannot serve it to — anonymous, no profile, no skills — so this is not gated
   *  client-side at all: every ordering is offered, and the feed explains the degraded
   *  one rather than withholding it.
   *
   *  The `null` matters because the DEFAULT depends on `q` (see defaultSortFor) while
   *  `q` changes under the ordering's feet. Storing the resolved default instead would
   *  make "the browse feed defaulted to newest" indistinguishable from "the caller
   *  asked for newest", so typing into the search box would carry `sort=posted_at`
   *  into a text search and date-order it — the exact outcome design.md rejects. Read
   *  this through effectiveSort, never directly. */
  sort: JobSort | null;
  /** Restricts `q` to a named subset of the search's usual fields (title, company,
   *  description, location) — set only by a title suggestion's `applyParts` call
   *  (see `filtersWithParts`), so the count it displayed (an exact-title-match count)
   *  approximates what searching actually returns instead of a much wider match
   *  across all four fields. `null`, not an empty array, spells "no restriction" —
   *  the same convention `salaryMin`/`postedWithinDays` use — and it is cleared
   *  whenever `q` is set independently (typing, the header's Enter, its clear
   *  button; see `FilterStore.setQuery`/`commitQuery`), so the restriction never
   *  outlives the search it was scoped to. Mirrors `q_fields` in
   *  internal/search/search/query_params.go. */
  qFields: string[] | null;
  /** Leave out the postings the signed-in caller has already opened — the cards the
   *  feed would otherwise dim. Serialized as `hide_seen=true`, true-or-absent.
   *
   *  A preference about the FEED, like `sort`, not a filter of the catalogue: the
   *  server reads it from the caller's own view history, which a saved search's digest
   *  has no equivalent of, so `savedSearchQuery` drops it and it is not counted as an
   *  active filter. A signed-out caller gets the unhidden feed (the server says so in
   *  `meta.ignored_params`), which is why the control is only offered signed in. */
  hideSeen: boolean;
  /** Change how work_mode composes with the location facets: instead of ANDing
   *  them (which narrows remote work to the home country), the server builds
   *  "remote anywhere, OR a selected physical mode within the location group"
   *  — remote from anywhere plus hybrid/on-site at home. Serialized as
   *  `work_mode_remote_anywhere=true`. The server applies it only when remote AND
   *  a physical mode are both selected AND a location facet carries a value
   *  (mirrors `remoteAnywhereActive`, see `appliesWorkModeRemoteAnywhere`), so
   *  every read here gates on the same condition: the checkbox only shows when
   *  it changes the answer, and only then it counts as an active filter. A saved
   *  search KEEPS it — the digest matcher replays the stored query through the
   *  same server builder, so the alert mails the set the subscriber saw. Mirrors
   *  `WorkModeRemoteAnywhereParam` in internal/search/search/query_filter.go. */
  workModeRemoteAnywhere: boolean;
  /** The remote arm's reach under the modifier (`work_mode_remote_regions` /
   *  `work_mode_remote_countries`): remote work within these, while the location
   *  facets bound the physical arm. Empty: remote from anywhere. Part of the
   *  modifier — the server reads them only while it is in force, and clearing the
   *  modifier clears them. */
  workModeRemoteRegions: string[];
  workModeRemoteCountries: string[];
}
