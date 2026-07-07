## Why

The most demoralizing part of a tech job search is not finding links — it is not knowing whether a posting is *real*. Evergreen reqs, talent-pool fishing, and reposts that fake a fresh date send applicants to burn effort on vacancies nobody is filling. Every other board trusts the source's posted date and cannot see through the trick. freehire uniquely keeps history (`jobs.created_at` = when *we* first saw a `(source, external_id)`), so it can tell an applicant "this 'posted 2 days ago' role has actually been open 240 days and was reposted 6 times" — a signal no single seeker, LLM, or competitor can compute.

## What Changes

- Add a deterministic **job-reality signal**: a small heuristic engine (`internal/jobreality`) that classifies each job as `fresh` / `stale` / `likely-evergreen` from history and text facts, mirroring the "never guesses" dictionary philosophy of `internal/location` / `skilltag` / `roletag` / `classify`.
- Signals (all deterministic, combined — one signal is weak, convergence is strong):
  - **True age** from `created_at` (not the gameable `posted_at`).
  - **Fake-freshness**: `posted_at` recent while `created_at` is old.
  - **Continuous-open duration** (`created_at → now`, `closed_at IS NULL`).
  - **Mass-posting**: count of open jobs for the same `(company_slug, normalized title)`.
  - **Evergreen text markers** ("always hiring", "talent community", "pipeline", RU equivalents) via a curated dictionary.
  - **Repost clustering**: how many distinct `external_id`s share one narrow role fingerprint over time → "reposted 6×".
- Persist a narrow **`role_fingerprint`** on the ingest write path (company + normalized title + description, **excluding** `posted_at`/url/slug) so reposts under new `external_id`s cluster. NOTE: `internal/jobhash` deliberately *includes* `posted_at` in `content_hash`, so `content_hash` cannot serve as the repost-identity key — a separate fingerprint is required.
- Expose the classification as a Meilisearch facet (`reality`) — filterable, but **not** hidden by default — and surface it in the job view as a **facts-backed badge** ("open 240 days · posted date bumped 6× · company keeps this role permanently"). The colored `likely-evergreen` verdict shows **only when multiple signals converge**, never on age alone — no defamatory single-signal accusation, low false-positive risk on genuinely hard-to-fill senior roles.
- Served **dict-only** and reconciled via `cmd/backfill-derive` + `make reindex`, exactly like the other derived facets.

## Capabilities

### New Capabilities
- `job-reality-signal`: deterministic classification of a job's likelihood of being a real, active opening (`fresh`/`stale`/`likely-evergreen`) from ingest history + text, served as a job-view field and a Meilisearch `reality` facet with a facts-backed badge.

### Modified Capabilities
- `source-ingest`: the `UpsertJob` write path additionally computes and persists a narrow `role_fingerprint` (excluding volatile fields) so reposts of the same role under new `external_id`s are countable.

## Impact

- **Schema** (`migrations/`): `jobs.role_fingerprint text` + a supporting index for the "distinct external_ids per fingerprint per company" count. Schema change → recreate the dev volume (known no-migration-runner seam; prod applied manually per deploy convention).
- **New package** `internal/jobreality` (+ dictionary, +tests).
- **`internal/jobhash` / ingest normalize path**: compute `role_fingerprint` alongside `content_hash`; wire into `UpsertJob` params (edit `internal/db/queries/jobs.sql` → `make sqlc`).
- **`internal/jobderive`** (or the backfill pass): derive `reality` at index build; `cmd/backfill-derive` re-derives on existing jobs.
- **`internal/search` + `internal/jobview`**: new `reality` served field + filterable facet attribute (NEW attribute → the usual "500 until first reindex" window applies).
- **`web/`**: reality badge on the job card + detail; optional `reality` filter chip in the filter modal.
- **Ops**: after ship — apply migration, run `cmd/backfill-derive`, then `make reindex`.
