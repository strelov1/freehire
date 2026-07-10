## Why

JustJoin.it jobs are ingested with an empty description: the adapter reads only the
`by-cursor` list endpoint, which omits the posting body. Every justjoin vacancy on
freehire therefore renders with no description and a broken, empty job page (e.g.
`/jobs/nodejs-software-engineer-e-mobility-energy-spyrosoft-qlna6pha`). The body lives
only in a per-offer detail endpoint (`GET /v1/offers/{slug}`), so the fix requires
detail requests — but justjoin serves ~20k live offers, so fetching detail for every
offer every crawl is prohibitively expensive.

## What Changes

- **`justjoin` adapter fetches per-offer detail to populate the description.** For an
  offer the catalogue does not already have, the adapter fetches `GET /v1/offers/{slug}`,
  puts the sanitized `body` HTML into the job description, and — since the detail is
  fetched anyway — fills the structured facets the platform states unambiguously
  (`requiredSkills` → `Skills`, `experienceLevel` → `Seniority`), mapped into freehire's
  controlled vocabularies (empty when unmapped). Category is left to the title dictionary:
  justjoin's `category` is a language/stack tag that does not pin a single role category.
- **Detail is fetched only for offers not already in the catalogue ("detail for new").**
  The pipeline passes a `seen(externalID)` predicate (backed by the set of already-ingested
  `external_id`s for the provider, one query per crawl) to a new optional adapter capability;
  an already-seen offer emits a list-only job (its `last_seen_at` is bumped, description
  unchanged). Steady-state crawls then hit detail only for the day's new offers.
- **Detail requests are bounded.** The adapter fetches detail with bounded concurrency and
  isolates a single offer's detail failure (it does not abort the crawl).
- **One-time backfill for the existing empty rows.** A run-once `cmd/backfill-justjoin`
  re-fetches detail for existing `source='justjoin'` rows and updates their description, so
  the ~20k already-ingested vacancies gain a body and re-index (their `content_hash` moves).

## Capabilities

### New Capabilities
- `justjoin-source`: the justjoin adapter's list+detail crawl — how it hydrates the
  description and structured facets from `/v1/offers/{slug}` for unseen offers, and the
  one-time backfill of existing rows.

### Modified Capabilities
- `source-ingest`: add an optional adapter capability that hydrates only postings the
  catalogue does not already have, driven by a provider-scoped seen-set the pipeline
  supplies — a bounded, opt-in seam that leaves every other adapter and the `Source.Fetch`
  signature unchanged.

## Impact

- **Code**: `internal/sources/justjoin.go` (new `FetchNew`, detail fetch + mapping),
  `internal/sources/source.go` (new `HydratingSource` optional interface),
  `internal/pipeline/pipeline.go` (type-assert the hydrating capability + seen-set port),
  `cmd/ingest/store.go` + `internal/db/queries/*.sql` (new `ExistingExternalIDs` query and
  a description-update query), new `cmd/backfill-justjoin/main.go`.
- **APIs**: none (backend ingest only).
- **Operations**: `cmd/backfill-justjoin` run once after deploy, followed by `make reindex`;
  first steady-state crawl is cheap (detail only for new offers).
- **Dependencies**: none new — reuses the existing shared HTTP JSON client and sqlc.
