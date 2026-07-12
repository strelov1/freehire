## Why

Aggregator sources (himalayas, gulftalent, the national portals, …) re-list jobs that
we already ingest first-hand from the company's own ATS. The existing repost dedup
(`ingest-content-dedup`) does NOT catch these: its `role_fingerprint` includes the
description, and aggregators rewrite the description, so an aggregator copy and its ATS
twin land in different clusters and are never collapsed. Measured on prod: ~9,442 open
aggregator postings have an exact-title ATS twin in the same company and compatible
country — each is currently indexed in Meilisearch, embedded, and LLM-enriched as if it
were a distinct job. That is wasted compute and a polluted search index for postings we
already serve, better, from the first-party source.

## What Changes

- A new suppression pass — run in the reindex reconcile, immediately after the existing
  role-duplicate recompute — marks an open **aggregator** posting as `duplicate_of` an
  open **ATS** posting when they share the same `company_slug`, the same normalized title,
  and a compatible country (country arrays overlap, or either is empty). The ATS posting
  is always the survivor; the aggregator copy is the duplicate.
- Suppressed aggregator copies inherit the existing `duplicate_of` treatment with **no
  downstream changes**: excluded from search/Meilisearch, from semantic embedding (and
  their vector removed), and from LLM enrichment — but still stored and reachable by
  their public slug (direct link), and still listed by the `job-cluster-copies` endpoint.
- The pass is **safe by construction**: only aggregator rows are ever suppressed (an ATS
  row is never demoted), only against an open canonical ATS row, and only within a
  compatible country (so a Singapore aggregator posting is never suppressed by a
  same-titled US ATS posting — the 7-Eleven false-merge). It never merges aggregator↔
  aggregator or ATS↔ATS; that stays the job of `role_fingerprint`.
- If the ATS twin later closes, the aggregator copy un-suppresses on the next reindex run
  and re-enters search/embedding/enrichment (idempotent failover, mirroring the existing
  role-duplicate recompute).
- No schema change: the aggregator/ATS distinction is sourced from
  `sources.AggregatorProviders()` (derived from the existing `aggregator()` registry
  markers) and passed into the query.

Scope is the **exact-title** key only. Aggregators that mangle titles (gulftalent's
Marriott copies drop the property suffix and leave `&amp;` undecoded) are a separate,
riskier fuzzy-title follow-up, out of scope here.

## Capabilities

### New Capabilities
- `aggregator-ats-dedup`: a reindex-reconcile pass that suppresses an aggregator job
  posting as a duplicate of a first-party ATS posting of the same company, title, and
  country, keeping the ATS copy canonical.

### Modified Capabilities
<!-- none — the duplicate_of exclusions (search, embed, enrich, by-slug, cluster-copies)
     already treat every duplicate_of row uniformly and are unchanged. -->

## Impact

- `internal/sources/` — add `AggregatorProviders()` enumerating the registry's
  `aggregator()`-marked providers.
- `internal/db/queries/jobs.sql` — new `CompaniesWithAggregatorPostings` driver +
  `SuppressAggregatorDuplicatesForCompany` per-company query; regenerate sqlc.
- `cmd/reindex/` — invoke the new suppression pass per company immediately after the
  existing `recomputeRoleDuplicates` (repost-collapse first, then cross-source
  suppression).
- No migration, no new column; reuses the existing `duplicate_of` column and every
  surface that already filters it.
