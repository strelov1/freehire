## Why

GitHub issue #2601: postings from Profession's two dedicated IT board sitemaps
(`itdev`, `itops`) are open, stored, described — and confirmed technical by the
source's own filing — yet are invisible in `/jobs/search`. Root cause traced in
code: Profession asserts no structured `category`/tech signal at ingest; when
the title dictionary also fails to resolve a category (it does, for about half
of these postings — the board already ingests them anyway, see
`profession.go`'s `professionITBoards` comment), `jobderive.deriveIsTech`
leaves `is_tech` unknown (`NULL`). `EnqueuePendingJobs` only queues rows where
`is_tech IS TRUE`, so such a job never enters the enrichment queue, its
`enrichment.category` never gets set, and `search.CategoryUnresolved` — which
gates both the incremental search-drain push and the full reindex rebuild —
stays permanently true. The job is excluded from search forever, not merely
until the next enrichment cycle.

## What Changes

- Profession's `itdev`/`itops` crawl (the only boards it ever crawls — every
  other category is refused before the sitemap is even read) now asserts a
  structured "confirmed technical" signal on postings it stores, the same way
  other adapters assert a structured `category` or `seniority` when their
  source states one directly. `jobderive` gains this as a new precedence input
  to its `is_tech` derivation, alongside the existing structured facets.
- `search.CategoryUnresolved` — the search-index quality gate meant to keep out
  the undifferentiated non-tech bulk a broad ATS crawl brings in (painters,
  stockers, drivers) — no longer excludes a job whose `is_tech` is confidently
  `true`. A confirmed-technical posting that lacks a specific sub-category is
  not undifferentiated bulk; excluding it defeats the gate's own stated
  purpose. This closes the gap for every source, not only Profession: any job
  whose title the tech dictionary confidently recognizes was already eligible
  for enrichment and is now also eligible for the index without waiting on it.
- A one-off backfill sets `is_tech = true` on the Profession `itdev`/`itops`
  rows already stored with `is_tech` unknown, so the fix reaches the ~300+
  postings stuck today and not only future crawls — mirroring how
  `backfill-clearance` and similar one-off passes handle a derivation change
  that a general `backfill-derive` sweep cannot reach because it re-derives
  from data the fix does not add a column for.

## Capabilities

### New Capabilities

(none — this is a defect fix within existing capabilities)

### Modified Capabilities

- `tech-classification`: adds a structured source signal that takes
  precedence in the tri-state `is_tech` derivation, for a source whose crawl
  scope itself confirms a posting is technical.
- `job-search`: documents (and narrows) the category-unresolved exclusion rule
  — a confirmed-technical job is no longer excluded from the index solely for
  lacking a specific category.

## Impact

- `internal/ingest/sources/source.go` (`Job` struct), `internal/ingest/sources/profession.go` (`detail`)
- `internal/ingest/pipeline/pipeline.go` (`normalizeJob`)
- `internal/job/jobderive/jobderive.go` (`Input`, `deriveIsTech`, `Derive`)
- `internal/search/search/document.go` (`CategoryUnresolved`)
- A new one-off backfill command (`cmd/backfill-profession-it-tech` or similar)
- Operationally: a full `make reindex` must run after the backfill, the same
  requirement `backfill-clearance` and `backfill-company-type-hint` carry,
  since `is_tech` is not hashed into `content_hash` and an incremental push
  alone would not reach these pre-existing rows.
