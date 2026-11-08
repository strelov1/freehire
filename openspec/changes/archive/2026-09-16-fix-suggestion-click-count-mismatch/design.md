## Context

See proposal.md for the symptom and root cause. Two mechanisms already exist and this
design reuses both rather than adding anything new:

- Meilisearch quoting: wrapping part of a `q` string in double quotes disables typo
  tolerance for those words and requires every one of them to be present. **It is not
  a contiguous-phrase match in this deployment**: the `jobs` index runs
  `ProximityPrecision: byAttribute` (`internal/search/search/client.go`, `#1637`),
  which gives Meilisearch only attribute-level, not word-level, distance data, so it
  cannot verify word adjacency. A quoted `q="founding engineer"` therefore matches a
  document containing both words anywhere in a searched field, in any order — this is
  documented and was measured, not assumed, in the sibling (not yet archived) OpenSpec
  change `search-q-field-scoping`, whose design.md also records that true contiguous
  matching would need `ProximityPrecision: byWord` plus a full catalogue reindex, with
  a measured indexing-cost regression (~1.7x slower bulk load, ~3.3x slower warm-index
  incremental push at 60k-document scale) — out of scope for that change and for this
  one.
- Field-scoped search: `q_fields` (`internal/search/search/query_params.go`,
  `QFieldsFromValues`, shipped by `search-q-field-scoping`) is already a public
  `/jobs`/`/jobs/search` param that restricts `q` to a named subset of
  `SearchableAttributes` via Meilisearch's `AttributesToSearchOn`, with no reindex.

Skill, category, and company suggestions are out of scope for the fix itself (see
proposal) because clicking them already sets an exact facet filter
(`skills=`/`category=`/`company_slug=`) against the same index their count was drawn
from — the mismatch is structural to title suggestions only, since `title` has no
facet and falls back to a free-text query (`apiSuggestions.ts`'s `facetFor` map).

## Goals / Non-Goals

**Goals:**

- Make a title suggestion's click-through result count closely approximate its
  displayed count, using only existing search mechanisms.
- Leave `cmd/build-suggestions` and the nightly dictionary build untouched — the count
  computation is not the thing being changed.
- Leave demand tracking (`search_queries`, `recordQuery`) counting a suggestion-driven
  search under the same key an equivalent typed search would use.

**Non-Goals:**

- Exact-phrase (word-adjacency) matching, or byte-exact parity between the suggestion
  count and the click-through total. Quoting a title-scoped query requires every word
  to be present, not that they are adjacent or in order — a title like "Engineer,
  Founding Team Lead" would match a quoted, title-scoped "founding engineer" query
  even though its normalised form was never counted in the "founding engineer"
  dictionary bucket. Closing the ~25x gap to a small residual is the bar; true
  adjacency matching is `search-q-field-scoping`'s explicitly deferred follow-up
  (`ProximityPrecision: byWord` + full reindex), not something to bundle into a fix
  that touches only how a suggestion is applied.
- Any change to skill/category/company suggestion behavior.
- Any change to `/jobs`'s general free-text search semantics for a query a visitor
  types directly (unscoped, un-quoted, multi-field) — only a suggestion-originated
  search changes shape.

## Decisions

**Decision: fix how a title suggestion is applied, not how its count is computed.**

Three directions were weighed:

1. **Chosen — narrow the click to match the count** (quote + `q_fields=title`).
   Cheap, reuses existing mechanisms, touches only the suggestion-application path.
2. **Rejected — widen the count to approximate the click.** Would require running a
   live (or periodically cached) full-text query per dictionary entry at build time —
   79,727 documents as of the last rebuild — turning a ~2-minute nightly job mining
   SQL aggregates into one issuing tens of thousands of search queries, for a number
   that would still drift between rebuilds exactly as it does today. Rejected on cost
   for a benefit direction 1 already delivers more cheaply.
3. **Rejected — UI-only, stop implying a promise.** Relabelling or dropping the count
   fixes the honesty problem but throws away real information the count carries
   ("this role exists in volume") that pointed 1 preserves. Reached for only if 1 had
   turned out infeasible.

**Decision: quote and scope in the frontend, strip the quoting in the backend's demand
recorder only.**

`apiSuggestions.ts` already owns turning a suggestion's parts into `plan.q` /
`plan.facets` (`applyParams`), so the title branch gains the quoting and a
field-restriction (`plan.qFields`). No new API surface — `q_fields` already exists
server-side. **This is one of two consumers of `ApplyPlan`; see the Addendum below for
the other, found during review.**

The one place this leaks is `recordQuery` (`internal/api/handler/search.go`), which
records `c.Query("q")` — the same raw string Meilisearch received — for demand
tracking. Left alone, a suggestion click for "Founding Engineer" would record the
literal string `"founding engineer"` (quotes included, since `suggest.Title` does not
treat `"` as a separator), permanently splitting demand tracking for that phrase into
a quoted bucket that never matches the dictionary's own key and never accumulates
enough count to surface. `recordQuery` therefore strips one matching pair of leading
and trailing `"` before calling `suggest.Title`, rather than teaching `Title` about
quotes: `Title` is also applied to raw mined posting titles (`cmd/build-suggestions`),
and a title that genuinely starts and ends with a quote character is a different case
that should not be silently unwrapped there.

**Decision: strip an embedded `"` in the suggestion text before wrapping it.**

A suggestion's title text is a mined, normalised catalogue title
(`suggest.build.displayTitle`), so an embedded double quote is unlikely but not
provably impossible. Wrapping `He said "wow" Engineer` naively would produce
`"He said "wow" Engineer"`, which Meilisearch would parse as more than one quoted
segment rather than one. The frontend strips any `"` from the text before wrapping it,
rather than escaping it — the same choice `check-alert-rules.py`'s comment makes
elsewhere in this codebase for a different reason (rewrite past the character, don't
escape it), and simpler than teaching the one call site Meilisearch's escaping rules
for a case that has not been observed to occur.

## Risks / Trade-offs

- **[Risk] A quoted, title-scoped query still isn't the dictionary's exact-match
  count** (see Non-Goals — no word-adjacency guarantee) → Mitigation: this closes the
  gap from ~25x to a small residual, not a guarantee of equality; no scenario in the
  spec claims exact parity, and the residual gap is explicitly named as
  `search-q-field-scoping`'s deferred follow-up, not a defect of this change.
- **[Risk] `q_fields=title` on a composed suggestion (title + company) narrows the
  title portion correctly but the URL now carries both `q_fields=title` and a
  `company_slug` facet filter — worth confirming these compose without surprising
  interaction** → Mitigation: `q_fields` only restricts which fields the free-text `q`
  matches against; it has no interaction with facet filters, which are a separate
  Meilisearch `filter` expression. Existing behavior, not new to this change.
- **[Risk] Forgetting the `recordQuery` fix silently pollutes demand tracking** →
  Mitigation: called out explicitly as its own spec scenario
  ("A suggestion-originated search still counts as ordinary demand"), so it has a
  dedicated test rather than riding along as an implementation footnote.

## Migration Plan

No data migration. No index rebuild. No deploy ordering concern — both the frontend
and backend halves of the change deploy together as an ordinary release, and neither
half is meaningful without the other (the frontend change alone would search
correctly but pollute demand tracking on every suggestion click; the backend
`recordQuery` fix alone has nothing to strip until the frontend sends quoted queries).
Rollback is a plain revert, since no persisted data shape changed.

## Addendum: the in-place list search was a second, unfixed consumer

Code review (before merge) traced `ApplyPlan`'s full consumer graph and found the
original design was wrong that `apiSuggestions.ts`/`browseTarget.ts` are "the single
place this logic lives": that is true only for the launcher, which always NAVIGATES to
`/jobs` and builds the URL from scratch. A title suggestion applied while already ON a
list page — `/jobs` itself, a company's job list (`CompanyView.svelte`), a role/country
page, or a collection — goes through a different path entirely:
`HeaderSearch.svelte`'s dropdown calls the list's own `suggest.applyParts`, which
`JobsView.svelte:360-366` wires to `FilterStore.applyParts(plan.facets, plan.q ?? '')`
— dropping `plan.qFields` on the floor. Worse, even fixing that one call site would not
be enough: `FilterStore` is built entirely on `JobFilters`/`filtersToParams`/
`filtersFromParams` (`facetModel.ts`), which had no field for a query-scoped
restriction, so nothing downstream — `scopedParams()` (the live search/facet-count
requests), `filters.params` (what `Pagination.svelte`'s links are built from) — had
anywhere to carry it even if `JobsView.svelte` passed it along. The result: the exact
count-mismatch bug this change exists to fix, reproduced on the more common of the two
suggestion-application paths, and reproduced again on page 2 of the ONE path
(launcher → `/jobs`) that did work, since pagination rebuilds its query from
`FilterStore`, not from the original navigation URL.

**Resolution: `qFields` becomes a real, first-class `JobFilters` field**
(`string[] | null`, matching the `salaryMin`/`postedWithinDays` null-means-unset
convention used throughout that type), rather than a value threaded around outside it.
This was chosen over the alternative of keeping it as a transient, non-persisted
side-channel (e.g. a field on `FilterStore` excluded from `.params`): making it a real
`JobFilters` field means every existing consumer of `filtersToParams`/`.params`/
`scopedParams()` — pagination, facet-count requests, saved searches, `localStorage`
persistence — carries it automatically, with no new plumbing at each call site. The one
risk this creates (a restriction outliving the search it was scoped to, e.g. if a
visitor types a brand new query into the same box afterward) is closed by having
`FilterStore.setQuery`/`commitQuery` — the only other writers of `q` — clear `qFields`
whenever `q` is set outside of `applyParts`. `filtersWithParts` (which `applyParts`
calls) replaces `qFields` rather than merging it, for the same reason: a suggestion
with no title part must not inherit a PREVIOUS suggestion's restriction.

A saved search or shared link created while a title-suggestion-driven restriction was
active now carries `q_fields` too (via `savedSearchQuery`/`filtersToParams`) — judged
correct, not a side effect to guard against: the restriction is part of what was
actually searched, so a saved search reproducing it exactly is the expected behaviour,
the same way it already reproduces `q` itself.

**Not covered, deliberately**: `FilterStore`'s SvelteKit/Svelte-5-runes dependency
(`urlSynced.svelte.ts`) means it cannot run under this project's plain-Node vitest
config (see `vitest.config.ts`'s own comment on why), so `FilterStore.applyParts`/
`setQuery`/`commitQuery` and the `JobsView.svelte` call site are not covered by an
automated test — the same pre-existing boundary the rest of `FilterStore` already sits
behind (no `filters.test.ts` exists for it today). The actual logic these three now
carry (setting/clearing `qFields`) is a thin pass-through to `filtersWithParts`, which
IS fully unit-tested in `facetModel.test.ts`. Closing that boundary for the whole class
would be a much larger, separate undertaking (a Svelte-aware test harness for every
`FilterStore` method) disproportionate to this change.

## Addendum 2: the quoting wrapper leaked into human-facing text

An independent code-review pass (run after the previous addendum) found that
`JobFilters.q`/`ApplyPlan.q` are read in three places as if they were plain,
human-typed text, and none of them were updated for the new quoting:

- `FilterSummary.svelte`'s "Search" chip renders `f.q` verbatim.
- `HeaderSearch.svelte`'s search box reconciles its displayed text from
  `target.value.q` (and, on first paint, the raw URL `q` param).
- `JobsView.svelte`'s `role_suggestion` analytics event falls back to `plan.q` for a
  title-only suggestion's `role` field.

After this change, all three would show or record `"Founding Engineer"` — literal
quote marks included — for a title suggestion, which is a real, visible regression:
the chip and the search box are meant to show what the visitor is searching for, not
a Meilisearch query-syntax detail, and the analytics `role` field is read downstream
as the role name, not as a quoted string.

**Resolution**: `displayQuery(q: string): string` (`facetModel.ts`) strips a matching
leading/trailing quote pair — the exact inverse of `quoteForTitleSearch`
(`apiSuggestions.ts`) — and is applied at every point `q` is read for a human (the
chip text, the search box's reconciled display value, the analytics fallback). It is
NOT applied to the value actually sent to the search API, the URL `q` param, or
`recordQuery`'s demand key — those three must keep the quoting wrapper (or, for
`recordQuery`, have it stripped by the separate `demandKey` normalisation) to work
correctly. `displayQuery` and `demandKey` solve the same shape of problem — "this
string may carry a query-construction artifact that must not leak into a different
context" — independently, on their own sides of the stack, because there is no
shared module between the Go backend and the Svelte frontend for them to share
through.

This was not caught by the original design or either implementation pass because
every test written up to that point exercised `q`/`qFields` as request-construction
inputs (what gets sent, what gets serialized to a URL) — none exercised what a human
sees. `displayQuery` itself is fully unit-tested; the call sites are the same kind of
thin, Svelte-dependent glue documented as untested in Addendum 1.

A follow-up sweep for every `track()` call reading `.q` (not just the three the
review named) found a fourth: `JobsView.svelte`'s separate `search` analytics event
(distinct from `role_suggestion` — it fires on every applied-filter change, not only
a suggestion pick) also read `filters.applied.q` raw. Fixed the same way. The
review's three findings were real and specific, but "every place this shape of bug
can occur" needed a grep across the whole `track()` surface, not just the sites
named — worth remembering for a similar leak next time: search for the PATTERN
(every reader of a field whose meaning just changed), not only the instances a
review happened to name.
