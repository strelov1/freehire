# Design

## Context

`GET /api/v1/jobs/search` already accepts everything this change surfaces:
`sort=posted_at`, `sort=created_at`, the two salary bounds, `sort=match`,
`posted_within_days`, and the `reality` facet. The gap is entirely in the web
client, in three different shapes:

1. **A control gated on the wrong thing.** `JobsView` computes
   `sortControlVisible = matchFilterAvailable && matchSortEnabled(env)`. The gate
   is correct for the `match` option — a "Best match" that ranks against no
   profile is worse than an absent option, which the code comment argues well —
   but it was applied to the whole select, taking `Newest` down with it.

2. **A control that misreports what the server does.** `newest` is
   `DEFAULT_JOB_SORT`, and `filtersToParams` omits the default. So under query
   text the request carries no `sort`, `searchSort` returns `nil`, and the engine
   ranks by relevance while the select reads "Newest".

3. **Controls that exist but are unreachable.** `posted` sits last in the rail's
   third section; `reality` has no rail entry at all.

The reporter's message — postings "years old" that "aren't in any particular
order" — is the visible surface of (1) and (2) together.

## Goals / Non-Goals

**Goals**

- A signed-out visitor searching by text can order the feed by posting date.
- The sort control's label matches the ordering the server actually applies.
- Freshness and the evergreen exclusion are reachable without opening a modal.
- A facet cannot again be declared, served, and unreachable without a test
  failing.

**Non-Goals**

- No endpoint, index, schema, or settings change. The API's behaviour is already
  what `job-search` specifies; only the client's model of it was wrong.
- No sort by salary. The two bounds are sortable and the reporter asked for it by
  name ("stipend"), but `enrichment.salary_min` is stored in the posting's own
  currency with no conversion. A descending sort would rank 90 000 JPY above
  80 000 USD — a control that looks authoritative and is not. It becomes viable
  paired with a currency filter or a normalised column; neither is in scope.
- No `updated_at` filter or sort. See "Deferred" below.

## Decisions

### Contextual default over a fourth sort value

**Chosen:** `defaultSortFor(q) = q ? 'relevance' : 'newest'`, and serialize only
when the selection differs from that default.

The endpoint's defaults are already contextual in exactly this way
(`search.go:212`: unknown/absent `sort` yields `posted_at:desc` when `q` is
empty, and no directive otherwise). Mirroring them means `relevance` never needs
a wire value the backend does not know — the client simply omits `sort`, and the
existing "omit the default" rule in `filtersToParams` does the work unchanged.

*Alternative considered:* teach the backend a literal `sort=relevance`. Rejected:
it adds a wire value, a handler branch and a spec change to express something the
absence of a parameter already expresses exactly.

*Alternative considered:* keep two options and always emit `sort=posted_at` for
`newest`. Rejected: relevance becomes unreachable, so every text search is
ordered by date and quality drops for the common case.

### The stored ordering is nullable: chosen, or not yet chosen

**Chosen:** `sort: JobSort | null`, with `null` meaning "no choice made" and the
default applied at read time.

The first draft stored the resolved default (`sort: 'newest'` for a fresh browse
feed). Code review caught what that costs: `setQuery` spreads the state and
overwrites only `q`, so nothing distinguishes "the browse feed defaulted to
newest" from "the caller asked for newest". Typing into the header search box
therefore produced `q=golang&sort=posted_at` — the primary search path, date-
ordered, which is the outcome the alternative below is rejected for. The AI filter
dialog reached the same state by the same route.

The second cost is quieter. `savedSearchQuery` is `filtersToParams(...)`, so
`sort` is part of the key that decides whether the live filters ARE the saved
search they came from (`SavedSearches.svelte`, `saveSearchAlert.ts`). A stored
query `q=go` parses to no ordering and serializes clean, but the typed state
serialized as `q=go&sort=posted_at` — so the saved search read as dirty, and
saving again created a duplicate saved search and a duplicate digest
subscription that would deliver byte-identical mail.

Nullability closes both directions with one rule instead of patching each entry
point that can change `q`.

### Collapse `relevance` → `newest` purely, not via an effect

`relevance` has nothing to rank against once the query is cleared. `JobsView`
already handles an analogous case with an effect (`minMatch` is reset when the
match slider disappears), but an effect is the wrong tool here: the serializer
needs the same answer, and two readers of one rule invite drift.

**Chosen:** one exported pure function in `facetModel.ts` that resolves a filter
state to its effective sort. The select's `value` and `filtersToParams` both call
it. Testable without a component harness, and it cannot disagree with itself.

### Visibility by option count, not by role

**Chosen:** render the select when it holds more than one option.

This falls out of the option rules rather than restating them: `newest` always,
`relevance` under query text, `match` under the existing gate. A signed-out
visitor on a bare list sees no select — correct, since the feed is already newest
and there is nothing to choose — and the same visitor with a query sees two.

The option list and the selected value are pure functions in `facetModel.ts`
(`sortOptionsFor`, `selectedSortFor`), not `$derived` in the view. Review made the
case: the first draft left them in `JobsView.svelte`, where `web/` has no
component-test harness, and both of the sort bugs review found were inside that
untestable region. The same argument that put `effectiveSort` in the model applies
to every rule that decides what the user sees.

`selectedSortFor` also answers a case the option rules alone do not. A shared
`?sort=match` link opened signed out resolves to an ordering that is not on offer,
and a `<select>` whose value matches no option renders **blank** — an empty control
over a live ordering. It names the ordering the endpoint will actually serve that
caller instead, which is honest rather than merely non-blank, because the endpoint
degrades `match` to exactly that. The same shape of bug applies to the freshness
select, where a day count from a shared link or the AI dialog need not be a preset;
`freshnessOptions` offers the live bound as a stop of its own.

### A toggle for evergreen, not a third select

The row already carries the total, two selects and (on the jobs list) the Swipe
entry; on a narrow phone a third select crowds it out. The `reality` facet's own
comment (`facets.ts:421`) states the common use is the single exclusion of
`likely-evergreen`, so a two-state toggle covers it. The full three-class facet
stays available in the modal, which is where a less common selection belongs.

Measured at a 390px viewport the toggle's WORD was wider than the select it
replaced, and the row ran 49px past the edge — the sort select clipped off-screen.
The toggle drops its word below `sm` and the icon carries it, which is what the
Swipe entry beside it already does and what `ListToolbar`'s own comment argues for.
`flex-wrap` on that row is the safety net under both: the children are sized by
their content (a longer count, a translated label, a future fourth control), and a
row that runs out of width must break rather than clip.

### `setNow`, not `setSoon`, for the freshness select

`setPostedWithinDays` debounces via `setSoon` because the modal control is a
dragged slider crossing intermediate stops. A select is a discrete commit and
must not sit behind a debounce, for the same reason `setSort` uses `setNow`. The
change adds a second setter rather than switching the existing one, so the
slider's behaviour is untouched.

## Risks / Trade-offs

- **[A shared link that predates this change reads differently.]** No link the
  client has ever produced carries `sort=posted_at` (the default was never
  serialized), so there is nothing to reinterpret. `sort=match` round-trips
  unchanged. → Covered by a deserialization scenario in the spec.

- **[The freshness value now has three writers: modal slider, page select, AI
  filter dialog.]** All three already write through the same store method, so the
  value cannot fork; the risk is only that a user misses one of them updating. →
  The filter summary already renders the active freshness chip, so the state is
  visible wherever it was set from.

- **[`ListToolbar`'s prop rename touches the company catalog.]** `CompaniesView`
  passes `sortControl` too. → Both call sites change in the same commit;
  `svelte-check` fails on a missed one rather than silently dropping the slot.

- **[Relaxing the desktop-row condition could double the total on a company
  page.]** The condition guards two things at once today. → The relaxed condition
  keeps the total gated on `showDesktopTotal` and gates only the controls on
  their own presence, which the spec's third scenario pins.

## Deferred: filtering or sorting on `updated_at`

Worth recording, because the column recently became meaningful and the reason it
still cannot be used is not obvious.

`RefreshUnchangedJob` (`internal/platform/db/queries/jobs.sql:368`) made a
re-crawl of an unchanged posting write `last_seen_at` and nothing else, so
`updated_at` now means "content last changed" rather than "last crawled" — the
comment at line 381 says so explicitly, and the jobs sitemap already serves it as
`<lastmod>` on that basis.

It is nonetheless absent from the Meilisearch document (`document.go` carries
`posted_ts`, `posted_at`, `created_at` and no more), so exposing it needs a
document field, a `FilterableAttributes`/`SortableAttributes` entry, a full
reindex, and a two-step release — settings live before the binary that queries
them, per the warning in `client.go`. A reindex is not cheap here: it is blocked
below a 45 GB disk floor and writes roughly 10 GB of skill vectors.

There is also a product question worth settling first. "Updated yesterday" is not
"posted yesterday": an evergreen posting whose description was edited would
surface as fresh, which is the ghost-posting problem the reporter described
rather than a fix for it. The `reality` facet this change surfaces answers that
question directly and needs no reindex, so it should be observed in use before
`updated_at` is added on top.

## Open Questions

None. Scope, control shapes and the deferred item were settled with the user
before this change was written.
