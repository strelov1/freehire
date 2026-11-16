---
type: Reference
title: Catalogue and search flow
description: Locate ingestion, enrichment and search ownership without guessing runtime state.
generated:
  by: codex
  at: 2026-11-16T23:17:31+00:00
sources:
  - resource: ../../internal/ingest/sources/wantapply.go
  - resource: ../../internal/ingest/pipeline/pipeline_write.go
  - resource: ../../internal/ingest/sources/providers/linkedin.md
  - resource: ../../internal/ingest/linkimport/native_write.go
  - resource: ../../internal/ingest/linkimport/linkedin.go
  - resource: ../../internal/api/handler/intake_linkedin.go
  - resource: ../../internal/api/handler/intake_sources.go
  - resource: ../../internal/ingest/contribution/contribution.go
  - resource: ../../cmd/ingest/native_ownership.go
  - resource: ../../internal/job/job/upsert.go
  - resource: ../../internal/ingest/sources/linkedin_related.go
  - resource: ../../internal/ingest/sources/scraplingtier.go
  - resource: ../../cmd/ingest/native_discovery_company.go
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

## LinkedIn as discovery

LinkedIn owns only postings with confirmed on-site application. External applications
resolve to their native ATS identity; verified transfers preserve the Freehire job id and
public slug. Employer website discovery onboards native boards for the existing scheduler,
so the ATS adapter owns subsequent updates and closure. See the
[LinkedIn provider reference](../../internal/ingest/sources/providers/linkedin.md) and
[ownership curator](../agents/workers/ops.md#promote-job-source).

The shared link intake accepts LinkedIn job URLs. Configured Scrapling transport is applied
only to its LinkedIn adapter; other URL resolvers keep their guarded client. It verifies
the application method before importing, attributes a discovered native board and its
region to the signed-in contributor, and queues unreadable or unverified destinations
without publishing them. A recorded board adopts its provider even after a failed posting
read, while an existing operator pause remains unchanged.

## WantApply regional inventory

WantApply global (`wantapply.com`, source `wantapply`) and Cyprus (`wantapply.cy`,
source `wantapply-cy`) have independent vacancy sets. Neither domain substitutes the
other. Original-posting boards bypass company-wide ingest coverage: another open
vacancy at an employer does not establish that its whole inventory is present.
Posting-level duplicate matching remains in place. See the
[adapter](../../internal/ingest/sources/wantapply.go) and
[coverage rule](../../internal/ingest/pipeline/pipeline_write.go).

## LinkedIn source identity and related discovery

New LinkedIn catalogue rows use the numeric posting id across search keywords and regions.
The shared writer preserves an existing public legacy row's id, slug and saved history by
its exact normalized native URL, including closed rows subject to ordinary rediscovery
policy. A LinkedIn-only URL index covers that closed history.

A completed bounded search may supplement its primary results with at most twenty
first-level recommendations from one verified public posting page. Each candidate is read
through the normal detail and application-ownership checks, with the same request limiter.
LinkedIn uses the existing Scrapling HTTP proxy route when configured. The provider
reference records the bounded access probes and their limits.

Ownership reconciliation accepts both numeric LinkedIn posting keys and legacy
keyword-scoped keys, so a later external-application transition can reach its native ATS.
