## Context

Ingest already computes `jobs.role_fingerprint` (`hash(company_slug|norm_title|
norm_description)`, migration 0003, `internal/jobhash.RoleFingerprint`) on every
upsert, indexed on `(company_slug, role_fingerprint)`. The job-reality signal already
aggregates each cluster (`RoleClusterCount`/`RoleClusterCountsAll` → repost_count,
mass_count) at index time and shows "reposted N×". What is missing is the **collapse**:
all reposts still render as separate cards, appear in search, and are enriched.

A spike also established the boundary: exact-content reposts share a fingerprint and
cluster cleanly, but reworded / cross-source copies get a different fingerprint (and
sit in the same similarity band as different roles), so they are out of scope here.

## Goals / Non-Goals

**Goals:**
- Show one canonical job per role cluster in list + search; skip reposts in enrichment.
- Reuse the existing fingerprint and cluster counts; do not disturb the reality signal.

**Non-Goals:**
- Reworded / cross-source near-duplicates (Track 2).
- Dropping inserts (the reality signal needs the rows) — collapse is read/enqueue-time.
- Backfilling existing rows in this change (a follow-up `cmd`).
- Changing `role_fingerprint` or the reality counting.

## Decisions

**Collapse at read/enrichment, not pre-insert.** The reality signal counts repost rows;
dropping inserts would break it. So every posting is still stored and counted, and a
canonical marker decides what the catalogue/search/enrichment surface.

**Stored canonical marker `jobs.duplicate_of` (bigint NULL), computed per cluster.**
A stored marker gives ONE consistent canon across list, search, and enrichment (three
independent query-time collapses would risk disagreeing). It matches the codebase's
denormalized compute-at-index pattern (skills/regions/role_fingerprint). Canon =
`min(id)` among the cluster's OPEN rows; the rest get `duplicate_of = canon.id`; canon
and empty-fingerprint rows stay null. `min(id)` is chosen over source-preference
because cross-source clusters are rare (rewrites diverge the fingerprint), so within-
source `min(id)` is deterministic, stable, and avoids coupling to the source registry.

**Recompute in the reindex path + a cron sibling.** The marker is refreshed by a set-
based `RecomputeRoleDuplicates` pass over open clusters, run where the role-cluster
counts are already recomputed (reindex) and on the recount cadence, so canon failover
(a closed canon → new `min(id)`) reconciles on the same schedule as the reality
counts. Incremental ingest push is best-effort consistent (a freshly-ingested repost
may show until the next recompute — the same eventual-consistency the facets have).

**Filters, not new code paths.** List and enrichment-enqueue add `duplicate_of IS
NULL`; reindex/incremental-push skip `duplicate_of IS NOT NULL`. Detail-by-slug is
unchanged (a repost stays reachable, like a closed job). Openings count reuses the
cluster open count already computed for the reality view.

## Risks / Trade-offs

- [Eventual consistency: a new repost shows until the next recompute] → Acceptable, and
  identical to how the reality counts and other denormalized facets already behave.
- [Canon churn] → `min(id)` is monotone and deterministic; a canon only changes when it
  closes, and then to the next `min(id)`. A test asserts stability across recomputes.
- [Genuinely distinct openings collapsed (929 per-branch roles)] → We surface the open
  count as "N openings"; the per-posting rows still exist and are reachable by slug.

## Migration Plan

New migration: `jobs.duplicate_of bigint NULL REFERENCES jobs(id)` + index. Manual prod
apply before deploy (initdb gotcha). Regenerate `internal/db` via sqlc. Deploy is code-
only after; the collapse takes effect once the recompute runs (reindex/cron). Rollback
= revert the query filters; the column is inert if unused.

## Open Questions

- Whether to expose `openings_count` as a distinct wire field or reuse the reality
  view's existing count (leaning reuse for MVP).
- Backfill/recompute of the whole catalogue on first deploy vs. letting the reindex
  pass populate it (leaning: run the recompute once on deploy alongside a reindex).
