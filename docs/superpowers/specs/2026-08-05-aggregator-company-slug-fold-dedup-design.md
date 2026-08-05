# Aggregator/ATS suppression: fold word-separator spelling variance in company_slug

## Context

`SuppressAggregatorDuplicatesForCompany` (`internal/db/queries/jobs.sql:562`) suppresses an
open aggregator posting under its canonical first-party ATS twin — same company, equal
normalized title, compatible country (shipped in `aggregator-ats-dedup`, see
`openspec/changes/archive/2026-07-12-aggregator-ats-dedup/design.md`). Both the `ats` and
`agg` CTEs filter `jobs.company_slug = sqlc.arg(company)`, and the Go driver
(`suppressAggregatorDuplicates` in `cmd/reindex/main.go:421`) runs the query once per
distinct `company_slug` found in `CompaniesWithAggregatorPostings`.

Found while investigating a reported duplicate
(`founder-associate-mba-graduate-cfoinsights-xpmm3mvp` vs
`founder-associate-mba-graduate-cfo-insights-ps7mck6u`): the same CFO Insights posting,
byte-identical description, arrived via `arbeitnow` (aggregator, raw company name
`"Cfoinsights"`) and `greenhouse` (ATS, raw company name `"CFO Insights"`). The stored
`company_slug` is computed as `normalize.Slug(company)`
(`internal/jobderive/jobderive.go:156`), which is plain transliteration + collapsing runs of
non-alphanumeric characters to a hyphen — it deliberately does not strip legal suffixes
(documented in `internal/normalize/slug.go:21`, "a noted future refinement"). The two raw
names slugify to `"cfoinsights"` and `"cfo-insights"` respectively. Because the suppression
pass requires **exact** `company_slug` equality, and it is scoped one exact slug at a time,
the aggregator copy is never compared against its ATS twin — not a lag-window issue (see
`hire-repost-marker-lag-window` memory), a permanent miss regardless of how many times
reindex runs.

This is scoped narrowly to the word-separator class of spelling variance (the one observed).
Legal-suffix variance (`"Cfoinsights Inc"` vs `"Cfoinsights"`) is a separate, already-noted
gap in `normalize.Slug`'s own doc comment, and `normalize.CompanyKey` (added for the harvest
own-ATS-board-discovery feature, PR #1413, `internal/normalize/company.go`) already exists to
close it — but is deliberately **not** pulled into this fix, to avoid scope creep on an
incident that doesn't demonstrate the legal-suffix case.

## Goals / Non-Goals

**Goals:**
- Suppress an aggregator posting against its ATS twin even when the two sources' raw company
  names slugify to different strings that agree once word-break hyphens are removed
  (`"cfoinsights"` vs `"cfo-insights"`).
- No change to the Go driver's per-company loop shape, locking, or error handling.
- No new column, no backfill.

**Non-Goals:**
- Legal-suffix normalization (`normalize.CompanyKey`'s other fold). Out of scope by explicit
  user decision — reopen only if a real incident demonstrates it.
- Any change to `company_slug`'s stored value, its role as a URL path segment, or
  `ListRoleClusterCopies`'s clustering key.
- Cross-source company identity resolution as a general project (rejected earlier in
  `hire-aggregator-ats-dedup-and-discovery` memory as not worth the website-resolution cost).

## Decisions

### Decision: fold both sides by stripping hyphens, inside the existing CTEs

Change both the `ats` and `agg` CTE filters in `SuppressAggregatorDuplicatesForCompany` from

```sql
WHERE jobs.company_slug = sqlc.arg(company)
```

to

```sql
WHERE replace(jobs.company_slug, '-', '') = replace(sqlc.arg(company)::text, '-', '')
```

(plus an explicit `AND jobs.company_slug <> ''` / `AND a.company_slug <> ''` guard added to
each CTE — neither existed before this change; they exist now so the query's predicate
provably implies the partial index's `WHERE company_slug <> ''` clause below, letting the
planner use it). Since `company_slug` is already
`normalize.Slug(name)` — lowercase, transliterated, non-alphanumeric runs collapsed to a
single hyphen, no legal-suffix stripping — the only residual difference between two sources'
slugs for the same company (absent legal-suffix noise) is exactly where word-break hyphens
landed. Stripping hyphens from both sides before comparing folds `"cfoinsights"` and
`"cfo-insights"` to the same key (`"cfoinsights"`) without touching the stored column, the
Go driver, or the per-company loop — the widening happens entirely inside the query each
invocation already runs.

- **Alternative — precompute `normalize.CompanyKey` equivalence classes in Go, widen the
  driver to an array param:** rejected for this incident. Reuses the exact helper already
  shipped for #1413 and would also close the legal-suffix gap, but requires a new
  broad-distinct-company_slug query every reindex cycle, a changed query signature, and
  re-verification of the per-company lock/error semantics under a "company" that is now a
  set of slugs. No evidence yet that the legal-suffix class causes real duplicates — YAGNI.
- **Alternative — resolve company identity at ingest time:** rejected. This is the general
  cross-source company-identity project already scoped out in
  `hire-aggregator-ats-dedup-and-discovery` as not worth the website-resolution cost.

### Decision: a new partial expression index, no backfill

Both existing indexes on `company_slug` (`jobs_company_slug_idx`,
`jobs_open_company_created_at_id_idx`, `migrations/0001_init.sql:746,756`) are plain btree on
the raw column — Postgres cannot use either for an equality on
`replace(company_slug, '-', '')`, and the suppression pass runs this filter once per company
in the reindex loop, so an unindexed seq scan per company is unacceptable. Add, following the established repo pattern for every prior index added to `jobs`
(`migrations/0013_jobs_open_role_cluster_idx.sql`, `0023`, `0026`, `0027`, `0033` — none of
which run `CONCURRENTLY` inside the tracked migration file, since Postgres flatly refuses
`CREATE INDEX CONCURRENTLY` inside a transaction block and this repo's `-- migrate:
no-transaction` escape hatch is reserved for the `ADD CONSTRAINT ... NOT VALID` / `VALIDATE`
split, not exercised here):

```sql
CREATE INDEX IF NOT EXISTS jobs_open_company_slug_folded_idx
    ON public.jobs (replace(company_slug, '-', ''))
    WHERE closed_at IS NULL AND company_slug <> '';
```

`IF NOT EXISTS` makes the migration safe to apply blocking on a fresh/empty volume (initdb,
CI, dev) where the cost is negligible. On the existing prod volume, per the same convention
as `0013`'s comment, an operator builds the equivalent index out of band, non-blocking,
*before* the tracked migration reaches prod:

```sql
CREATE INDEX CONCURRENTLY jobs_open_company_slug_folded_idx
    ON public.jobs (replace(company_slug, '-', ''))
    WHERE closed_at IS NULL AND company_slug <> '';
```

so that when `migrate` later applies the tracked file, `IF NOT EXISTS` finds the index
already present and is a no-op — the live table is never blocking-locked. No new column, no
backfill-derive step: Postgres builds the expression index from existing data either way.
Before merge, `EXPLAIN` the modified query against a local Postgres with representative data
to confirm the planner actually switches to an Index Scan on the new index rather than
falling back to a seq scan — a mismatched expression between the index definition and the
query predicate is a well-known way for this kind of index to silently go unused.

## Risks / Trade-offs

- **Coincidental fold collision between two distinct companies** (e.g. `"Met A"` →
  `met-a` → folded `meta`, colliding with `"Meta"` → `meta`) — not a new risk class: the same
  fold is already accepted in production by `normalize.CompanyKey`/`SameCompany` (PR #1413),
  documented there as "mild... but not fuzzy." The suppression pass's title-equality gate
  (the `matches` CTE) still has to agree independently, so a slug-fold collision alone cannot
  suppress two genuinely different postings — it only admits more candidates into the title
  comparison that already gates the actual merge.
- **Legal-suffix class stays open** — by explicit scope decision. If a `"Cfoinsights Inc"` vs
  `"Cfoinsights"` duplicate is reported later, extend via the `normalize.CompanyKey`
  equivalence-class approach (rejected alternative above), not by expanding this SQL fold.
- **Index not adopted by the planner** — caught by the pre-merge `EXPLAIN` check, a deploy
  blocker rather than a silent regression.

## Migration Plan

One migration: `CREATE INDEX IF NOT EXISTS ... jobs_open_company_slug_folded_idx`, applied by
initdb/CI/dev directly; on prod an operator builds the `CONCURRENTLY` equivalent out of band
before the tracked migration runs, matching `0013`'s convention. The SQL query and its
generated sqlc code change alongside it. No data backfill. Rollback is reverting the code
(the query change) and
dropping the index; no persisted state this pass writes needs unwinding beyond what
`SuppressAggregatorDuplicatesForCompany` already handles (idempotent, `IS DISTINCT FROM`
guard, re-evaluated every run).

## Testing Plan

- Extend `internal/db/aggregator_dedup_integration_test.go` (`-tags=integration`) with a case:
  an aggregator row (`company_slug = "cfoinsights"`) and an ATS row
  (`company_slug = "cfo-insights"`), equal normalized title, expect the aggregator row's
  `duplicate_of` to resolve to the ATS row's id.
- Keep the existing exact-match cases passing unchanged (the fold is a superset of exact
  equality, since a slug with no hyphens folds to itself).
- `go vet -tags=integration ./...` before push, per `AGENTS.md`.
