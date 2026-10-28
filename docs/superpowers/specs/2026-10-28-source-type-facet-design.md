# Source-type facet (`source_type`, `source_type_exclude`)

## Why

An API customer asked to keep search focused on original postings: "only ATS platforms
and company career sites", or "everything except aggregators". Today the only lever is
`source=` / `source_exclude=` over ~400 provider names, which nobody can maintain by hand.

## What

A new closed-vocabulary search facet, `source_type`, with four values:

| Value        | Meaning                                                    | Examples                         |
|--------------|------------------------------------------------------------|----------------------------------|
| `ats`        | a multi-tenant ATS platform (per-tenant board)             | greenhouse, lever, workday       |
| `company`    | one employer's own careers site, or a vacancy published here by a verified employer | apple, google, `employer` |
| `aggregator` | a multi-company job board that republishes postings        | adzuna, whatjobs, bayt           |
| `social`     | postings read out of community channels                    | telegram, hackernews             |

Like every facet it gets `source_type_exclude` and `source_type_mode` for free, appears in
`/jobs/facets`, works in saved searches and alerts, and is validated by `meta.ignored_values`.

Sources outside the four (`manual`, `weblink`) carry no `source_type` on the document. They
are reachable only through `source=` and are excluded by any positive `source_type` filter.

## Where the rule lives

`sources.ProviderKind` (internal/ingest/sources/classify.go) stays the ONE classifier — the
`/sources` page, the status page and now search all ask it.

- New `KindSocial`, set by a new `social()` adapter marker (hackernews) and by name for the
  non-adapter `telegram` source. `social` wins over `aggregator`.
- The non-adapter `employer` source classifies as `KindCompany`.
- The vocabulary lives in `internal/dict/vocab` (`SourceTypeValues`), so the search package
  and the generated web contracts can read it; a test in `sources` holds every `ProviderKind`
  answer to that vocabulary plus `KindOther`.
- Behaviour that keyed on `KindAggregator` (the ingest coverage gate) switches to the
  `aggregator` adapter MARKER, so hackernews keeps being gated exactly as today.

## Index

- `JobDocument` gains `source_type` (omitted when empty).
- `search.FromJob` takes the source type as a parameter — the same reasoning as `desc`: a
  writer that forgot would not fail, it would silently drop the posting out of the filter.
  The three callers (`cmd/reindex`, `cmd/search-drain`, `internal/ingest/linkimport`) resolve
  it once per run from `sources.Taxonomy()`.
- `source_type` joins `filterableAttributes`. Rollout order is mandatory: patch the live
  index settings, then ship the binary, then a full reindex. Until the reindex the facet is
  empty, so a positive filter returns nothing; `search-settings-drift` reports the gap.

## Web

- A `Source type` pills control in the filter modal (excludable), next to `Source`.
- Labels: ATS, Company site, Aggregator, Social.
- `ProviderKind` in `web/src/lib/types.ts` and `StatusBoard`'s order gain `social`.

## API contract

`web/static/openapi.yaml` documents `source_type` / `source_type_exclude` on the jobs search
and facets endpoints. Additive only.

## Testing

- `sources`: ProviderKind for hackernews → social, telegram → social, employer → company;
  every answer is in vocab or `other`.
- `search`: `source_type=ats&source_type=company` builds the OR filter; `source_type_exclude`
  builds the AND-NOT; an unknown value is reported in `ignored_values`.
- `FromJob` writes the passed type.
