## Why

The NEOGOV adapter (governmentjobs.com / schooljobs.com) is list-only: it stores the
short teaser snippet from the listing card (`li.list-item .list-entry`) as the job
description. The full posting body — Minimum Qualifications, Examples of Work,
Supplemental Information — lives on the per-vacancy detail page the adapter never
fetches, so every NEOGOV job on freehire ships a truncated description
(e.g. `/jobs/social-worker-5-a-...` shows only the opening paragraph).

## What Changes

- The NEOGOV adapter fetches each listing card's detail page and stores the full
  job body (the `#details-info` / `.fr-view` container) as sanitized HTML, instead of
  the listing snippet.
- The detail fetch is a plain GET (no `X-Requested-With` header — unlike the listing,
  the detail page is server-rendered) and is bounded so one board cannot issue
  unbounded concurrent requests, per the existing source-ingest requirement.
- A failed or empty detail fetch degrades to the listing snippet rather than dropping
  the job, so a transient detail failure never produces a blank description.
- **No backfill script.** Existing NEOGOV rows are corrected in place by the next
  normal `cmd/ingest sources/neogov.yml` crawl: `UpsertJob` overwrites a stored
  description with the non-empty full body, and the resulting `content_hash` change
  re-pushes the doc to the search index. Operators may run that crawl once
  post-deploy to backfill immediately instead of waiting for cron.

## Capabilities

### New Capabilities
- `neogov-source`: the NEOGOV career-site adapter's ingest contract — board shape,
  listing pagination, and detail-fetched full description.

### Modified Capabilities
<!-- none: source-ingest already permits per-posting detail fetch and requires sanitized-HTML descriptions; this change realizes those for NEOGOV without altering the general contract -->

## Impact

- `internal/sources/neogov.go` (+ `neogov_test.go`): add per-card detail fetch and
  full-body extraction; the `neogovHTTP` port already exposes the needed text GET.
- No schema, migration, or API change. No new env.
- Ingest cost: NEOGOV crawls change from one request per board to one per board plus
  one per posting (bounded), the same N+1 shape as other detail-fetching adapters.
