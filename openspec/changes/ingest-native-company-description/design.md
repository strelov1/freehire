## Context

See `proposal.md` for motivation and the spike results (Greenhouse's board
metadata endpoint carries real employer-authored text for a meaningful share
of boards; Workable's equivalent field exists but was empty in every sample
checked; Ashby and Pinpoint expose no company-level field at all).

Relevant existing state:

- `Source.Fetch(ctx, e CompanyEntry) ([]Job, error)` (`internal/ingest/sources/source.go`)
  returns one slice of postings per board; there is no existing per-board
  (as opposed to per-posting) return value.
- `UpsertYCCompany` (`internal/platform/db/queries/companies.sql:278`)
  already implements the fill-gap upsert this change needs for
  `tagline`/`company_info`/`company_info_at`, but it also unconditionally
  overwrites YC-owned columns (`subindustry`, `year_founded`,
  `employee_count`, `hq_country`) that this change has no data for and must
  not touch or null out.
- How a `companies` row first comes to exist for a newly-crawled slug is not
  a single obvious call site (`SyncCompaniesFromJobs` exists but is a
  rekey/rebuild tool, not clearly part of the steady-state per-job ingest
  path). Rather than depend on that ordering, this change's own write uses
  its own `INSERT ... ON CONFLICT`, safe regardless of whether the row
  already exists.

## Goals / Non-Goals

**Goals:**
- Capture Greenhouse's board-level `content` field once per board crawl and
  fill-write it into `tagline`/`company_info` under the existing gap-fill
  rule.
- Keep the adapter contract change generic enough that a future adapter
  (Workable, or any platform later found to expose similar data) can adopt
  it without another contract change.

**Non-Goals:**
- Implementing Workable's side of this now — its field was empty in every
  spike sample; revisit if a larger sample shows real coverage.
- Any change to how `Company` (the display name) itself is derived — this is
  purely about the free-text description.
- Retroactively backfilling companies already crawled before this ships —
  every board is re-crawled on its normal schedule, so existing companies
  pick this up on their next ordinary ingest run with no separate pass
  needed (unlike the Wikipedia backfill, which has no "will run again soon
  anyway" crawl loop underneath it).

## Decisions

**1. Shape of the contract addition: a new optional per-board fetch, not a
field on every `Job`.**

`Job` already repeats `Company` (the name) on every posting because the type
is inherently per-posting. A company-level description is a per-*board*
fact, so cramming it onto every `Job` in the batch would mean N identical
copies for a board with N postings, and an awkward "which one wins if an
adapter is inconsistent" question. Instead, `Source` gains a second,
optional method:

```go
type CompanyDescriber interface {
    CompanyDescription(ctx context.Context, e CompanyEntry) (string, error)
}
```

An adapter implements it only if its platform supports it (Greenhouse does;
most don't). The pipeline type-asserts for the interface after `Fetch`,
calls it once per board, and treats a `("", nil)` return (or the interface
not being implemented) identically: no description, no write. This keeps
every other adapter's `Fetch` signature and behavior completely unchanged.

*Alternative considered:* add `CompanyDescription string` to the `Job`
struct, populated identically on every posting in a board's batch. Rejected
— duplicates a per-board fact N times, and invites a future adapter bug
where two postings in the same batch disagree.

**2. One new, narrow SQL query — not a reuse of `UpsertYCCompany`.**

`UpsertYCCompany` overwrites `subindustry`/`year_founded`/`employee_count`/
`hq_country` unconditionally because YC is their only writer; this ingest
path has no such data and must not null those columns out for a company
`UpsertYCCompany` hasn't touched yet. A new query,
`FillCompanyDescriptionFromIngest` (name TBD at implementation time), does
the same `INSERT ... ON CONFLICT (slug) DO UPDATE` shape but touches only
`tagline` (`COALESCE(NULLIF(companies.tagline, ''), EXCLUDED.tagline)`),
`company_info` (`EXCLUDED.company_info || companies.company_info`, same
gap-filling key-merge), and `company_info_at`. On insert of a genuinely new
slug it sets `is_reference = false` (unlike YC's `true`) since this company
is arriving with a real crawled job, not as a reference-only row.

**3. Fetched once per board crawl, not cached across runs.**

Greenhouse's board-metadata endpoint is cheap (one small JSON document) and
ingest already re-crawls every board on its normal schedule, so there is no
need for a separate cache or staleness policy — each ingest run's fetch is
naturally the freshest available copy, and the fill-gap write means a board
whose employer later fills in a previously-blank `content` field picks it up
on its next ordinary crawl with no extra machinery.

## Risks / Trade-offs

- **[Risk]** An extra request per Greenhouse board increases crawl load and
  request budget for a platform ingest already treats as latency-sensitive.
  → **[Mitigation]** One small JSON request per board per crawl cycle (not
  per posting), which is negligible next to the per-posting detail requests
  several other adapters already make; no new per-run budget knob is
  expected to be needed, but the task list includes checking this against
  Greenhouse's actual crawl frequency before rollout.
- **[Risk]** Greenhouse's `content` field can itself change (an employer
  edits it) and the fill-gap rule means a once-empty field that later gets
  filled will be picked up, but a field that changes from one real value to
  another different real value will NOT be — same staleness trade-off
  already accepted for `tagline` across all three sources. → **[Mitigation]**
  Acceptable; matches existing behavior, not a new gap introduced by this
  change.
- **[Risk]** The `content` field is raw employer-authored HTML and may
  contain boilerplate, formatting cruft, or promotional content unlike the
  terse, curated taglines the other two sources produce. → **[Mitigation]**
  Sanitize through the same `sanitizeHTML` helper adapters already use for
  posting bodies before storing, and store it in `company_info.summary`
  (prose-length) rather than the short `tagline` field, which stays reserved
  for the shorter, curated-style values.

## Migration Plan

1. Add the `CompanyDescriber` interface and the pipeline hook that calls it
   once per board when an adapter implements it.
2. Implement it for Greenhouse.
3. Add the narrow fill-gap SQL write and its Go plumbing.
4. Ship behind no flag — this is strictly additive (a company only ever
   gains a `tagline` it didn't have) and runs as part of the existing,
   already-scheduled Greenhouse ingest, so there is no separate rollout
   step; verify on the next scheduled Greenhouse crawl by re-checking a
   known-content board (e.g. `coinbase`) via `GET /api/v1/companies/coinbase`.

No rollback beyond the general fill-gap-write rollback already described in
the Wikipedia backfill's design: clearing the specific columns by hand if
ever needed, since writes never overwrite another source.
