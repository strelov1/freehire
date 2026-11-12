# internal/search

The Meilisearch index topology, the incremental drain into it, saved searches, the facet snapshot, and intent parsing.

**Layer 6 of 8.**

May import: `platform`, `dict`, `ai`, `identity`, `candidate`, `job` — and itself.

Must NOT import: `application`, `engage`, `ingest`, `api`.

`application` share this layer, and the ban runs both ways: two blocks that can see each other are one block under two names.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Packages

`embedprogress` `facetidx` `facetsnapshot` `savedsearch` `search` `searchdrain` `searchintent`
`suggest`

## What the jobs index withholds (`search.Withheld`)

Beyond closed, duplicate and private rows, an open posting stays out of the index when it
has no resolved category, no body, or a source that has not shown it for longer than its
window (`search.UnseenWindow`, 14 days; per source from `sources.UnseenWindows`, passed in
by the caller because this block may not import `ingest`). `Withheld` is the ONE predicate
`cmd/reindex`, `cmd/search-drain` and `linkimport` ask; `RefreshCompanyFacets` mirrors it in
SQL and the similar-jobs list applies the unseen half. The unseen rule is evaluated when a
document is written, never at query time: ordinary unchanged liveness refreshes do not
update its indexed `last_seen_at`, so a query-time filter on that value would freeze and
hide live postings. Visibility is reconciled incrementally as well as by a full rebuild:
`cmd/reindex` in marker-only mode (`REINDEX_DEDUP_ONLY=1`) queues postings that crossed the
unseen window through time alone (`enqueueTimeCrossings`), and `RefreshUnchangedJob`
queues an unchanged posting listed again after its withholding threshold. The search
drain then applies `Withheld` to remove or restore the document; content writes also
queue work, and `linkimport` applies the rule to its writes. These transitions require
the corresponding worker runs; a stopped full-rebuild timer alone does not prevent
incremental reconciliation. See
[docs/agents/job-lifecycle.md](../../docs/agents/job-lifecycle.md) for which sources are
exempt and why.

## A dropped filter says so (`meta.ignored_params`, `meta.ignored_values`)

Any endpoint whose answer WIDENS when it does not understand a query param reports the
params it did not read in `meta.ignored_params` (omitted when there are none). Refusing them
would break saved searches and shared links that still carry a retired facet; staying silent
is what let a mistyped `country=it` pass for a search of Italy. `search.UnknownParams` owns
the SEARCH filter's vocabulary and only that, so an endpoint reading a different one says so
itself — `/companies` through `UnknownCompanyParams`, `/jobs` (which reads no filter at all)
through its own two-param list.

Its value-level twin is `meta.ignored_values`: a closed-vocabulary facet value outside
`internal/dict/vocab` (`work_mode=foo`) typically still narrows — to nothing — and
`search.IgnoredValues` names it rather than letting `total: 0` pass for an empty market. Not
every unknown value narrows to nothing: a true-or-absent boolean reads anything but `false`
as true (`requires_clearance=flase` filters FOR clearance, `requires_clearance_exclude=flase`
excludes the postings that require it), while an unknown `_exclude` value on an ordinary
equality facet excludes nothing; all are named all the same.

The same rule decides what may be SUBSCRIBED to: `search.Narrows` refuses a saved search with
no text and no surviving filter, rather than reporting it, because nobody reads a digest's
meta. It checks only that a filter exists, not that it removes anything:
`source_exclude=notarealsource` passes and digests the whole catalogue.

## The suggestions dictionary (`suggest`)

The search box's completions, in a SEPARATE Meilisearch index (`suggestions`) built
offline by `cmd/build-suggestions`.

It exists because the facet dictionaries cannot answer what people type. 8,680
postings are titled "Product Owner" and no facet value spells it — the closest is the
`product` specialization, labelled "Product Manager" — so the box answered a question
about real postings by renaming the person asking. Titles are the vocabulary the
market actually uses.

**Not a facet on `jobs`.** `title` is searchable there but not filterable, distinct
titles number in the millions, and `MaxValuesPerFacet` truncates a facet's
distribution. Its smallness is also the performance argument: the box queries it per
settled keystroke, and doing that against 8M documents would put a per-keystroke query
on the index serving the whole site.

Three things are worth knowing before changing it:

- **`Title` is ONE function**, applied to mined titles, to typed queries recorded in
  `search_queries`, and to the phrase recognition. A second copy drifts, and the drift
  looks exactly like a suggestion nobody ever searches for.
- **The floor runs AFTER normalisation.** "Product Owner", "product owner" and "PRODUCT
  OWNER" are three catalogue rows and one suggestion; a floor applied before merging
  drops a title that clears it comfortably after.
- **Recognition is exact, completion is fuzzy**, and the split is deliberate. A
  mistyped phrase must fall into the fragment where Meilisearch forgives it; consuming
  it as recognised means the typo is never corrected. Reimplementing the fuzzy half
  here would be the second matcher this package was built to retire.

An empty query returns nothing, and that is a boundary. What an empty box offers is the
filter modal's curated category order, which lives in `web/src/lib/filterSections.ts`
and is checked there against the category vocabulary at compile time.

## Facet engine (facetd)

`cmd/facetd` is a second engine for facet counts, totals and distinct counts (`Client.Facets`,
`Client.DistinctCount`) on requests WITHOUT query text; text queries always go to Meilisearch.
The router is `search/route.go`; wiring is `cmd/server/facet_engine.go`. Design:
`openspec/changes/facetd/design.md`.

- **Modes.** `meili` (default): Meilisearch only. `shadow`: Meilisearch answers, the engine is
  called in the background (bounded concurrency) and differences are logged and counted.
  `facetd`: the engine answers; any error, timeout, unsupported request or not-ready index falls
  back to Meilisearch.
- **Env (hire-api).** `FACETD_URL` (unset = no router, today's behaviour), `FACETD_TIMEOUT`
  (per engine call, default `150ms`), `SEARCH_FACET_ENGINE` (start mode; invalid value fails the start).
- **Flip at runtime, no deploy.** Redis key `freehire:search:facet-engine` = `meili|shadow|facetd`,
  polled every 10s. Rollback: `redis-cli SET freehire:search:facet-engine meili`. `DEL` returns to the
  start mode; an invalid value is logged and ignored; a Redis failure keeps the last mode.
- **Metrics** (`freehire_search_facet_*`): `requests_total{engine,mode,kind}`,
  `engine_seconds{engine,kind}`, `fallbacks_total{reason,kind}` (a rising `error`/`timeout` rate is
  the signal to roll back), `shadow_diffs_total`, `shadow_errors_total`, `shadow_skipped_total`.
  Run `shadow` until `shadow_diffs_total` stays flat before moving to `facetd`.
