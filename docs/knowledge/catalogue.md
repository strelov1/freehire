---
type: Reference
title: Catalogue and search flow
description: Locate ingestion, enrichment and search ownership without guessing runtime state.
generated:
  by: codex/gpt-6.1-sol
  at: 2026-11-15T18:33:40-03:00
sources:
  - resource: ../../cmd/reindex/summary.go
  - resource: https://github.com/strelov1/freehire/blob/c0fc3518d5f25c66f39b069c943fe7a1fab840b9/AGENTS.md
  - resource: https://github.com/strelov1/freehire/blob/c0fc3518d5f25c66f39b069c943fe7a1fab840b9/internal/ingest/AGENTS.md
  - resource: https://github.com/strelov1/freehire/blob/c0fc3518d5f25c66f39b069c943fe7a1fab840b9/internal/search/AGENTS.md
---

The crawl reads boards from PostgreSQL's `boards` catalogue. Source adapters live
in `internal/ingest/sources`; a directory of YAML board files is not the catalogue.
Postings are normalized and upserted with `(source, external_id)` as the dedup key.

Background enrichment uses `enrichment_outbox`; embedding uses `semantic_outbox`.
Search indexing has its own drain and rebuild paths. PostgreSQL stores the
catalogue, while Meilisearch serves search documents.

## Investigating a missing posting

1. Read [ingest ownership](../../internal/ingest/AGENTS.md) and the relevant adapter.
2. Read [search visibility rules](../../internal/search/AGENTS.md): an open row
   alone does not guarantee index eligibility. Category, body, duplication,
   privacy and source liveness matter.
3. Read [job lifecycle](../agents/job-lifecycle.md) for closing and liveness rules.
4. Find the relevant command in [workers](../agents/workers.md), then inspect its
   implementation and the deployed schedule through [operations](operations.md).

Use current source and observations for a diagnosis; these documents do not
assert queue health, current catalogue size or timer status.

## Summary duplicates

The existing fuzzy marker pass handles conservative WhatJobs summary matching as
well as near-identical descriptions. Its rules and operational limits are in
[the search worker reference](../agents/workers/search.md#whatjobs-summary-duplicates).
This describes the checkout implementation, not a deployed deduplication run.
