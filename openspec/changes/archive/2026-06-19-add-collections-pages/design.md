## Context

freehire serves a faceted job feed from Meilisearch. Every existing facet
(`skills`, `regions`/`countries`, `work_mode`, `seniority`/`category`,
`posting_language`, …) follows one pattern: a deterministic **source fact**
derived at ingest from the job itself, stored in a `jobs` column *beside* the
`enrichment` JSONB, served dict-only by `jobview.FromRow`, and indexed as a
filterable Meili attribute. Geography also denormalizes a company-level key
(`company_slug`) onto `jobs` "so a company's jobs are a single-table filter (no
join)".

Curated collections (`yc`, `bigtech`) are a new kind of editorial axis: a fact
about the **company**, not the job. They cannot be derived from a job's text or
its ATS source — Airbnb and Dropbox are YC companies but hire through their own
Greenhouse boards, so `source = workatastartup` captures only current early-stage
YC startups, not "YC companies". Membership therefore needs explicit data,
sourced from external datasets and matched onto our `companies`.

## Goals / Non-Goals

**Goals:**
- A company can belong to ≥1 curated collection; this drives a job-feed landing
  page per collection that reuses the existing faceted search unchanged.
- Membership is populated from external data (YC open dataset; a hand list for
  Big Tech) by an idempotent, re-runnable import worker.
- Adding a new collection is one registry entry plus a member resolver.
- Reuse the established facet plumbing — no new search path, no bespoke feed.

**Non-Goals:**
- Collection CRUD UI / per-collection cover art / arbitrary user-defined
  collections. The set is curated and fixed in code (one registry entry each).
- Domain-based company matching (matches by normalized name only).
- Stamping `jobs.collections` at ingest time (relies on periodic propagation +
  reindex, like every other dictionary facet).
- A harvest pass over the unmatched collection companies to discover new ATS
  boards (a high-value follow-up: the datasets double as a coverage worklist).

## Decisions

**1. Membership is a company fact; the facet is a denormalized job copy.**
`companies.collections TEXT[]` is the source of truth for membership;
`jobs.collections TEXT[]` is a denormalized copy that feeds the Meili facet.
This mirrors `company_slug`'s denormalization exactly and keeps the search path a
single-table read. *Alternative considered:* filter the feed at query time with
`company_slug IN [...slugs]` (no new job column, instant membership changes) —
rejected because a collection like YC can match hundreds of companies, bloating
the filter string and coupling the page to a separate company-list fetch.

**2. `collections` is NOT part of `jobderive`.** Every other facet is derived
from the job's own `Input` (title/desc/location). Collections come from the
company, external to the job, so they are propagated by a plain SQL copy
(`UPDATE jobs SET collections = c.collections FROM companies c WHERE
jobs.company_slug = c.slug`), run by the import worker after it writes membership.
This is simpler than a re-derive and keeps `jobderive` purely text-driven.

**3. Static registry, not a DB table.** `internal/collections` holds the fixed
set: each entry is `{slug, title, description, source}` where the source is either
a static slug list or a `Dataset{URL, Parse}`. The current set is `yc`,
`techstars`, `european`, `ai`, `mag7`, `bigtech`, `unicorn`, `fortune500`; adding
one is a single entry. A `company_collections` join table or a `collections`
definition table would be over-engineering for a hand-curated, code-owned set;
`companies.collections` as a `TEXT[]` mirrors `jobs.skills` and needs no extra
schema. The web side mirrors the registry as a small contracts list (noted seam:
later generated via `gen-contracts`).

**4. Import worker resolves, writes, propagates.** `cmd/import-collections` is a
run-once-and-exit worker (same shape as the other `cmd/` workers). Per collection:
- `yc`: fetch the open `yc-oss` dataset (JSON: name, website, batch, status),
  normalize each company name via `normalize.Slug`, match to existing
  `companies.slug`; log unmatched for manual follow-up.
- `bigtech`: a hand-coded slug list in the registry.
It recomputes the managed tags idempotently (a full rewrite of the two tags it
owns), then runs the SQL propagation onto `jobs.collections`, then prints a
`make reindex` reminder. It only manages tags it knows about, so manual edits to
other tags (future) survive.

**5. Search & wire shape.** `jobview` and the search document gain
`Collections []string json:"collections"`; `"collections"` is added to
`FilterableAttributes` (`internal/search/client.go`) and the query-param→filter
map (`internal/search/query_filter.go`). The `/collections/[slug]` page calls the
existing `/jobs/search` with `collections=<slug>` locked on; the `/collections`
index reads per-collection open-job counts from the `collections` facet
distribution in one search call.

## Risks / Trade-offs

- **Partial YC coverage from name matching** → Accept for a curated showcase;
  unmatched companies are silently omitted, the worker logs them, and domain
  matching is a documented later refinement.
- **Rare same-name false positives** (two distinct companies normalizing to the
  same slug) → Low frequency; tolerable for an editorial surface, surfaced via the
  unmatched/ambiguous log.
- **Staleness between imports** (a job ingested for a tagged company after the
  last propagation carries no tag until the next run) → Acceptable; YC batches
  refresh a few times a year and the import + reindex is cron-schedulable, same
  caveat as every dictionary facet.
- **New binary must be wired into the Dockerfile** (build + COPY) or it won't ship
  — known gotcha for every new `cmd/`.
- **`cmd/reslug` wipes membership** → it re-keys companies via
  `DeleteOrphanCompanies` + `SyncCompaniesFromJobs`, which recreates rows with
  `collections = '{}'`. Any reslug run must be followed by `cmd/import-collections`
  (then a reindex) to restore membership; note this in the reslug runbook.

## Migration Plan

1. Ship migration adding `companies.collections` and `jobs.collections` (both
   `TEXT[] NOT NULL DEFAULT '{}'`). On a persistent DB the migration is applied
   manually (no versioned runner yet).
2. Deploy the code (jobview/search facet, web pages, new binary).
3. Run `cmd/import-collections` once to populate membership + propagate.
4. `make reindex` to surface `jobs.collections` in Meili.
- **Rollback:** the facet is additive; the pages degrade to empty if membership
  is unpopulated. Reverting is dropping the two columns and the routes — no data
  loss for existing jobs.

## Open Questions

- Final Big Tech slug list (curate during implementation against actual
  `companies` slugs present in prod).
- Exact `yc-oss` endpoint/snapshot to pull (pin a stable JSON URL).
