## Why

`companies.industries` holds at least two vocabularies at once — a 400-company
production sample carries `AI` beside `Artificial Intelligence`, `FinTech` beside
`Fintech`, and 159 of its 235 distinct values come from a taxonomy we do not own.
Nothing filters on the column: it is absent from the Meilisearch company index's
filterable attributes, so it reaches users only as a display field.

The coarse `domains` facet cannot absorb the need. It is a fact about a job (the
periodic facet recompute rebuilds it from `jobs.enrichment`), its vocabulary is
deliberately 20 broad verticals, and in that same sample `other` was its largest
bucket at 42% of tagged companies. Those companies do have a describable industry
— an external company-directory dump labels them `Medical Devices`,
`Renewable Energy`, `Wealth Management`.

## What Changes

- Introduce a curated alias→canonical industry dictionary, dict-only: a tag
  outside the dictionary emits nothing. Canonical values are lowercase slugs, so
  they survive a filter URL.
- Route every writer of `companies.industries` through that dictionary.
- **BREAKING (data)**: a one-time pass rewrites the existing column through the
  dictionary. Values outside it are dropped, and the column becomes dict-only.
  The pre-change column is backed up first.
- Stop `UpsertYCCompany` replacing `tagline`, `company_info` and `industries`
  wholesale. It merges instead: an existing tagline outranks the YC one-liner,
  JSONB keys fill gaps without overwriting, industries union.
- Add a run-once worker that normalizes the column and merges an external
  company dump into it, reporting every tag the dictionary failed to recognize
  so the vocabulary has a growth path.
- Expose `industries` as a filterable company facet in Meilisearch, the
  `/companies` API and the companies page.

## Capabilities

### New Capabilities
- `industry-vocabulary`: the curated alias→canonical industry dictionary, its
  dict-only resolution rule, and the canonical/display split.

### Modified Capabilities
- `company-info`: `industries` becomes a controlled vocabulary rather than free
  text, and the company-info write path stops overwriting values owned by other
  sources.
- `yc-company-enrichment`: the yc-oss mapping's industries pass through the
  dictionary, and the upsert merges rather than replaces.

## Impact

- New: `internal/industrytag`, `cmd/import-company-industries`.
- Modified: `internal/db/queries/companies.sql` (`UpsertYCCompany`, two new
  queries), `cmd/import-yc`, `internal/search/company.go`,
  `internal/handler/companies.go`, `cmd/gen-contracts`, `web/src/lib/facets.ts`.
- Operational: a new filterable Meilisearch attribute needs a company reindex
  before it answers; until then, filtering on it fails. The reindex must not run
  while another one is in flight — Meilisearch serves a single task queue.
- Data: `companies.industries` is rewritten for every row that has values.
