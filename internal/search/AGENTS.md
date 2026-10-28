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

`embedprogress` `facetsnapshot` `savedsearch` `search` `searchdrain` `searchintent`
`similarjobs` `suggest`

## What the jobs index withholds (`search.Withheld`)

Beyond closed, duplicate and private rows, an open posting stays out of the index when it
has no resolved category, no body, or a source that has not shown it for longer than its
window (`search.UnseenWindow`, 14 days; per source from `sources.UnseenWindows`, passed in
by the caller because this block may not import `ingest`). `Withheld` is the ONE predicate
`cmd/reindex`, `cmd/search-drain` and `linkimport` ask; `RefreshCompanyFacets` mirrors it in
SQL and the similar-jobs list applies the unseen half. The unseen rule is evaluated when a
document is written, never at query time: the crawl's liveness refresh pushes nothing to
`search_outbox`, so an indexed `last_seen_at` would freeze and hide live postings. A
posting therefore leaves and returns with the full rebuild, and **turning the rebuild timer
off freezes the rule** — nothing new is hidden or restored until it runs again. See
[docs/agents/job-lifecycle.md](../../docs/agents/job-lifecycle.md) for which sources are
exempt and why.

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
