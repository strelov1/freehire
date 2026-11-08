## Context

`jobs.company` is a free-text column written by `UpsertJob`, the single write
path used by the (future) ingestion pipeline. There is no company entity, so
companies cannot be listed or shown with their jobs. The user's hard constraint:
companies must be a real table, but the common read paths must avoid joining
`companies` with `jobs`. The guiding principle for this change is **maximum
simplicity** — build the smallest correct thing and leave clearly marked seams.

Schema is applied via Postgres initdb from `migrations/`, which runs each file
once on first volume init only. This is the first schema change after init.

## Goals / Non-Goals

**Goals:**

- A `companies` table with a natural `slug` key (no surrogate id).
- A denormalized `jobs.company_slug` link key beside the display `company`.
- Join-free reads for company list and company detail.
- The write path keeps the company table populated automatically.

**Non-Goals:**

- Denormalized `job_count` counter (counts computed on the fly for now).
- Company metadata (logo, website, description).
- Stripping legal suffixes (LLC, Inc, ООО) during normalization.
- A versioned migration runner.
- Backfilling `company_slug` for rows in a pre-existing database (dev recreates
  the volume).

## Decisions

**1. Natural `slug` key, no surrogate id.**
The company's identity is its normalized name. A surrogate id would add an
indirection the user explicitly rejected and buys nothing here. `slug TEXT
PRIMARY KEY`, `name TEXT NOT NULL`, plus `created_at` / `updated_at` to match the
`jobs` convention. Alternative (surrogate id + FK) rejected: more columns, forces
a join or a `company_id` lookup on the hot path.

**2. Denormalize the link key onto jobs (`company_slug`), keep `company` as
display name.**
Carrying the normalized key on the job row makes "a company's jobs" a single-
table indexed filter (`WHERE company_slug = $1`) instead of a join, and keeping
the raw `company` name means job cards need no join either. This is the
denormalization the user asked for, applied to the link rather than to company
metadata. Index: `CREATE INDEX ON jobs (company_slug)`.

**3. Link by convention, no foreign key.**
No `FOREIGN KEY (company_slug) REFERENCES companies(slug)`. A real FK would force
insert ordering (company row before job) and add rigidity for little gain at this
scale; the write path already writes both. Simplest coherent choice. Trade-off: a
job could in principle carry a `company_slug` with no company row — prevented in
practice because the write path always upserts the company first.

**4. Normalize name → slug in Go, in the write path.**
A small Go function (lowercase, trim, collapse whitespace, strip punctuation —
no legal-suffix stripping yet) produces the slug. The source writes both
`company` and `company_slug`. Chosen over a DB trigger/generated column for
visibility and testability, and because the pipeline is the natural home for
normalization. Empty name → empty slug → no company upsert.

**5. Single atomic write for job + company.**
`UpsertJob` upserts the company (`INSERT ... ON CONFLICT (slug) DO UPDATE SET
name = EXCLUDED.name, updated_at = now()`) and the job together. Prefer one SQL
statement via CTE so the existing "one write = one job" property holds; fall back
to a transaction wrapping two queries if the CTE hurts readability. Skip the
company upsert entirely when the slug is empty.

**6. Counts computed on the fly.**
`GET /api/v1/companies` computes job counts with an aggregate at query time
rather than maintaining a counter. Aggregating over jobs per catalog page is
cheap until the catalog is hot; the `job_count` counter is a noted seam.

## Risks / Trade-offs

- **Slug collisions** (two distinct companies normalize to the same slug) → merged
  under one company. Acceptable for an aggregator MVP; suffix-stripping is
  deliberately omitted to keep collisions rare. Mitigation seam: refine
  normalization later.
- **Orphan `company_slug`** (no FK) → only possible if something other than the
  write path inserts jobs. Mitigation: the write path always upserts the company
  for a non-empty slug.
- **On-the-fly counts** get slow once the catalog is large/hot → introduce the
  `job_count` seam at that point.
- **Migration applies on fresh volume only** → for dev, `docker compose down -v
  && make up`; documented, not solved here.

## Migration Plan

1. Add `migrations/0002_companies.sql`: create `companies`, add
   `jobs.company_slug`, add the `company_slug` index.
2. Recreate the dev volume (`docker compose down -v && make up`) since initdb
   does not re-run on an existing volume.
3. Update queries and run `make sqlc`; commit generated code.

Rollback: drop the new endpoints/queries and `migrations/0002_*`; `company_slug`
is additive and can be left in place or dropped on a volume recreate.
