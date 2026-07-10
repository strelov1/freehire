## Context

`companies` is slug-keyed (normalized name, no surrogate id) and rows are born from jobs
(`jobs.company_slug`); `DeleteOrphanCompanies` removes any company no job references. The
existing facet columns (`company_types`, `company_sizes`, `countries`, `domains`, `regions`)
are job-derived — `RefreshCompanyFacets` recomputes them as the distinct union across a
company's open jobs (noisy per-job LLM enrichment + geo dictionaries). `companies.countries`
means *where the jobs are*, not the company HQ.

An external company info dataset (~158k companies), harvested out-of-band into a local
JSONL artifact, carries authoritative per-company facts: headcount, founding year, HQ
country, organization type, industries, tagline, and lower-coverage funding/stock/parent
data. This change lands the data model, a one-time loader, and company-detail exposure.

## Goals / Non-Goals

**Goals:**
- Authoritative company-info columns on `companies`, independent of the job-derived facets.
- Enrich existing companies and import unmatched ones as visible reference rows that
  auto-upgrade when a job later appears for them.
- Keep the loader and schema free of any reference to the dataset's origin.

**Non-Goals:**
- No recurring refresh / cron worker (one-time backfill; re-run manually).
- No `industries` search facet or frontend beyond company-detail exposure (Phase 2).
- No domain-based fuzzy matching in v1 (slug-only).
- No LinkedIn/social URLs (absent from the dataset; no fabricated guesses).

## Decisions

**New columns, not a new table (variant B).** Company info live on `companies` directly,
with `is_reference` distinguishing imported reference rows. *Alternative — a separate
`company_company info` table joined at read time* — was rejected: it adds a join to every
company read for no invariant benefit once `DeleteOrphanCompanies` is guarded. The columns
are independent of the job-derived facets, so there is no clobber between the backfill and
`RefreshCompanyFacets`.

**Reference rows via a boolean flag.** `is_reference BOOLEAN DEFAULT false`; the only
behavioral coupling is `DeleteOrphanCompanies` gaining `AND NOT is_reference`. A reference
row that later gets a job simply has `job_count > 0`; the flag may stay true (it only gates
deletion). *Alternative — a `has_jobs` column or provenance enum* — is more surface for no
gain, and a provenance string would name the source (disallowed).

**Slug-only matching in v1.** The loader normalizes each record's company name via
`internal/normalize` (the catalogue's slug rule) and matches on `slug`. *Alternative —
secondary domain match (the record's website ↔ `domains[]`)* — deferred until the miss rate
is measured; the loader logs matched-existing vs inserted-reference counts to inform that.

**Low-fill extras in JSONB.** parent, subsidiaries, activities, funding, stock, and the
website (~10–20%, website ~99%) go into a `company_info` JSONB rather than sparse columns.
The website goes to the JSONB — not the job-derived `domains[]` that `RefreshCompanyFacets`
owns — so it is available without clobbering.

**Run-once host worker, not the moderator API.** `cmd/backfill-company-info <file.jsonl>`
follows the `cmd/backfill-derive` shape (needs `DATABASE_URL`), streaming the file and
upserting via one new sqlc query `UpsertCompanyInfo`. A 158k-row import over the
HTTP moderator API would be needlessly slow for a one-time job with direct DB access.

## Risks / Trade-offs

- **Low slug match rate for existing companies** → the loader logs matched vs inserted
  counts; if enrichment coverage is poor, add the domain secondary match in a follow-up.
- **Catalogue grows ~158k rows; deep `OFFSET` pagination on the company list may slow** →
  the list query is unchanged here (reference rows sort to the tail); keyset pagination is a
  later fix if needed (the sitemap already uses keyset).
- **`is_reference` invariant relaxation touches orphan cleanup** → covered by an integration
  test asserting `DeleteOrphanCompanies` skips reference rows; no other code assumes
  "every company has a job".
- **Idempotency / partial reruns** → the upsert is `ON CONFLICT (slug)` writing only
  company-info columns, so a re-run or resume rewrites the same values without duplicates.

## Migration Plan

1. Ship migration `0042_company_info.sql` (additive columns + GIN index on
   `industries`); apply to the persistent DB before deploying code that reads the columns
   (no versioned runner — manual `psql`, per project ops).
2. Regenerate `internal/db` with `make sqlc`; deploy the binary carrying
   `cmd/backfill-company-info` and the guarded `DeleteOrphanCompanies`.
3. Run the backfill once against the local JSONL artifact.
4. Rollback: the columns are additive and nullable; reverting code leaves them dormant.
   Reference rows can be removed with `DELETE FROM companies WHERE is_reference` if needed.

## Open Questions

- Slug match rate against the live catalogue — measured on first run, decides whether the
  domain secondary match is worth adding.
