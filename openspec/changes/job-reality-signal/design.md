## Context

freehire already derives several deterministic facets from a job row (`internal/location`, `skilltag`, `classify`, `roletag`) and serves them dict-only through `jobview.FromRow` into a Meilisearch facet. This change adds a *history-derived* signal rather than a text-only one: it reads `jobs.created_at` / `posted_at` / `closed_at`, a repost-cluster count, a mass-posting count, and an evergreen-text dictionary hit, and folds them into a single `reality` class. The distinguishing asset is `created_at` — the moment freehire first saw a `(source, external_id)` — which no consumer of the source's `posted_at` can reconstruct.

Constraints: sqlc-only DB access (edit `internal/db/queries/*.sql` → `make sqlc`); migrations apply via initdb (dev volume recreate; prod manual per deploy convention; no migration-runner yet); deterministic "never guesses" dictionary philosophy; additive and surgical; derived-facet changes reach existing jobs only via `cmd/backfill-derive` + `make reindex`.

## Goals / Non-Goals

**Goals:**
- Classify each job `fresh` / `stale` / `likely-evergreen` deterministically, with `likely-evergreen` gated on signal *convergence*.
- See through fake freshness using `created_at`, and cluster reposts under new `external_id`s via a narrow `role_fingerprint`.
- Serve the class as a job-view field + `reality` Meilisearch facet, with facts-backed evidence for the badge.
- Fit the existing derive/backfill/reindex machinery with no new worker.

**Non-Goals:**
- No company-level "this employer posts fake jobs" verdict — only per-posting, per convergence.
- No LLM/ML classifier; enrichment's `enrichment.*` is never a source for this facet.
- No new liveness/probe mechanism (the existing `liveness_strikes` orphan path is untouched).
- No default hiding of `likely-evergreen` in the feed (facet is opt-in) — chosen framing is "middle": colored verdict only on convergence.

## Decisions

**1. A dedicated `role_fingerprint`, not `content_hash`.** `jobhash.Of` includes `posted_at` (and url/slug), so `content_hash` changes on a reposted-fresh duplicate and cannot cluster reposts. Add a narrow fingerprint over `company_slug + normalizedTitle + normalizedDescription`. Alternative considered: strip `posted_at` from `content_hash` — rejected because `content_hash` is the *change* signal for incremental indexing and must stay sensitive to `posted_at`; the two fingerprints have opposite jobs.

**2. Persist the fingerprint on the write path; compute the repost *count* at derive/index time.** The fingerprint is a stored column (`jobs.role_fingerprint`) written by `UpsertJob`; the "distinct external_ids per (company_slug, role_fingerprint)" count is a query run during derive, not a stored counter (avoids write-time fan-out and keeps `UpsertJob` a single-row upsert). A partial index on `(company_slug, role_fingerprint) WHERE closed_at IS NULL` backs the count.

**3. The class is computed in a pure `internal/jobreality` package.** Input is a small struct (age from `created_at`, `posted_at`, `closed_at`, repost count, mass-posting count, evergreen-text bool) → output `{Class, Evidence}`. This keeps the thresholds/convergence rule unit-testable in isolation, mirroring how `classify`/`roletag` are pure over their inputs. The scoring rule: `fresh` when age ≤ freshWindow and no evergreen signal; `likely-evergreen` when ≥ N of {old-age, repost≥k, mass-posting≥m, evergreen-text} converge; else `stale`.

**4. Evidence is served, not just the class.** `jobview` carries `reality: {class, ageDays, repostCount, massPostingCount, fakeFreshness}` so the SPA renders facts, not a bare label — this is the legal/UX guard behind the "middle" framing.

**5. Reality is compute-at-index, NOT a stored derived facet — because it is time-dependent.** Unlike geography/skills/seniority (invariant given the content), reality's age term (`now - created_at`) changes with the clock, so a stored class would go stale between derives (a `fresh` job silently becomes `stale`). It is therefore computed at index build in `search.FromJob` and served as a top-level `JobDocument` field, exactly like `Roles` (`roletag.Derive`) and `PostedTS` already are — no column, no `jobderive` entry, no `backfill-derive` participation for the class. What DOES get stored is only `role_fingerprint` (a stable content fingerprint, an input to the counts). The repost/mass counts are precomputed **once per reindex** as a `map[(company_slug, role_fingerprint)] → {repostCount, massPostingCount}` and passed into `FromJob`, so a full rebuild stays one aggregate pass, not N per-row queries. The incremental ingest push computes counts per new/changed job via a bounded single query. The single-row detail read (`jobview`) computes reality from the row plus one small count query. A dictionary/threshold change reaches existing jobs on the next `make reindex` alone (no `backfill-derive` needed for the class); only a `role_fingerprint` normalization change needs a fingerprint re-backfill.

## Risks / Trade-offs

- **False `likely-evergreen` on honest hard-to-fill roles** → convergence gate (never age-only) + facts-backed badge + opt-in facet (no default hide). Tunable thresholds live in one pure package.
- **Repost count needs history to be meaningful** → on a fresh catalogue the count is 1 for everything; the signal strengthens as reposts accumulate. Acceptable: the other signals (true age, fake-freshness, text) work from day one.
- **New filterable Meili attribute 500s `/jobs` until the first reindex** (known window) → ship binary, then `backfill-derive` + `reindex` in the documented order.
- **Fingerprint normalization drift** (title/description cleaning) could split or over-merge reposts → keep normalization narrow and deterministic, cover with tests; a later normalization change is a `role_fingerprint` re-backfill.
- **Schema change** (`role_fingerprint`) → dev volume recreate; prod migration applied manually *before* the new binary deploys (per the deploy/unapplied-migration convention).

## Migration Plan

1. Migration: `ALTER TABLE jobs ADD COLUMN role_fingerprint text;` + partial index.
2. `make sqlc` after editing `UpsertJob` to accept/write `role_fingerprint`.
3. Ship binary; apply migration in prod before deploy.
4. Backfill `role_fingerprint` for existing rows (one-off derive), then `make reindex` — the reindex computes `reality` at index time (no `backfill-derive` step for the class, since it is not a stored facet).
5. Rollback: the facet is additive; dropping the column and reverting the binary leaves the catalogue intact (soft, no data loss).
