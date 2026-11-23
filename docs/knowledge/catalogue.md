---
type: Reference
title: Catalogue and search flow
description: Locate ingestion, enrichment and search ownership without guessing runtime state.
generated:
  by: codex
  at: 2026-11-23T13:46:18Z
sources:
  - resource: ../../docs/agents/public-figures.md
  - resource: ../../internal/platform/db/queries/insights_role_demand.sql
  - resource: ../../internal/ingest/sources/providers/jobsacuk.md
  - resource: ../../internal/ingest/sources/providers/charityjob.md
  - resource: ../../internal/dict/classify/AGENT-REFERENCE-LANGUAGES.md
  - resource: ../../tools/gen-contracts/vocab.go
  - resource: ../../internal/job/job/upsert.go
  - resource: ../../internal/platform/db/queries/jobs_linkedin_identity.sql
  - resource: ../../cmd/harvest-linkedin/register.go
  - resource: ../../harvest/linkedin-queries.yml
  - resource: ../../internal/ingest/sources/linkedin_search_options.go
  - resource: ../../internal/ingest/sources/wantapply.go
  - resource: ../../internal/ingest/pipeline/pipeline_write.go
  - resource: ../../internal/ingest/sources/providers/linkedin.md
  - resource: ../../internal/ingest/linkimport/native_write.go
  - resource: ../../internal/ingest/linkimport/native_resolve.go
  - resource: ../../internal/ingest/linkimport/native_employer.go
  - resource: ../../internal/ingest/boardcatalog/repository.go
  - resource: ../../internal/ingest/linkimport/linkedin.go
  - resource: ../../internal/api/handler/intake_linkedin.go
  - resource: ../../internal/api/handler/intake_sources.go
  - resource: ../../internal/ingest/contribution/contribution.go
  - resource: ../../cmd/ingest/native_ownership.go
  - resource: ../../internal/job/job/upsert.go
  - resource: ../../internal/ingest/sources/linkedin_related.go
  - resource: ../../internal/ingest/sources/scraplingtier.go
  - resource: ../../cmd/ingest/native_discovery_company.go
  - resource: ../../cmd/ingest/native_discovery.go
  - resource: ../../internal/ingest/boardresolve/company.go
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

The [weekly hiring-demand figures](../agents/public-figures.md#weekly-hiring-demand)
use a PostgreSQL snapshot of publication counts by specialty.

## Title classification

The [title dictionaries](../../internal/dict/classify/AGENT-REFERENCE-LANGUAGES.md)
resolve role categories and seniority during ingestion. The [web contract generator](../../tools/gen-contracts/vocab.go)
builds role picker search terms from the shared alias tables.

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

Search the ingest journal for `linkedin_ats_unresolved` to investigate missing ATS
coverage. The reason distinguishes an unresolved Apply destination, an unresolved career
site, a missing employer website, and a detected provider without a usable adapter.
The event includes the LinkedIn posting ID and stores only the hostnames from URLs.
Use observed career hosts as leads when checking coverage.

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
policy. A LinkedIn-only URL index covers that closed history. The shared transaction
serializes writes for the same numeric LinkedIn posting before resolving its legacy key;
concurrent discoveries therefore preserve one identity and its original public slug.

A completed bounded search may supplement its primary results with at most twenty
first-level recommendations from one verified public posting page. Each candidate is read
through the normal detail and application-ownership checks, with the same request limiter.
LinkedIn uses the existing Scrapling HTTP proxy route when configured. The provider
reference records the bounded access probes and their limits.

Ownership reconciliation accepts both numeric LinkedIn posting keys and legacy
keyword-scoped keys, so a later external-application transition can reach its native ATS.

## Recurring LinkedIn harvest

`harvest-linkedin --register-searches harvest/linkedin-queries.yml` previews the existing
keyword/market worklist; `--apply` registers pending search boards. The regular ingest
scheduler then owns those searches, using the existing oldest-run-first admission when a
run cannot reach every board. Worklist page/depth and age settings travel with each board; registration can explicitly
override these budgets for new searches using `--pages` and `--jobage`. Repeated registration preserves existing live operator settings. See the
[worker reference](../agents/workers/ingest.md).

## Jobs.ac.uk and CharityJob catalogue identity

[Jobs.ac.uk](../../internal/ingest/sources/providers/jobsacuk.md) uses the posting reference as
its external ID and verifies coverage of the unfiltered listing before reading details.
[CharityJob](../../internal/ingest/sources/providers/charityjob.md) uses its own job ID, verifies
the page sequence and reads descriptions through the crawl-host browser. Both preserve the
listed identity when a detail is temporarily unreadable.
