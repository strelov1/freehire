## Why

Reed (reed.co.uk) is the UK's largest job board, but it is an aggregator we can't
crawl freely: its site (`/jobs/`) and API (`/api/`) are both `Disallow`ed in
robots.txt, and the Jobseeker API is gated behind a (free) API key. With a key,
the API exposes the real employer apply URL (`externalUrl`), full description, and
a precise employer — good data for net-new UK coverage. We want only the IT/tech
slice (freehire is an IT aggregator), which the API can't filter by sector, so we
scope it with a curated set of IT keywords.

## What Changes

- Add a `reed` source adapter over the Reed Jobseeker API, registered **only when
  `REED_API_KEY` is set** (the one other keyed source besides `usajobs`). The key
  is a secret in the environment, never in a board file.
- The adapter is **boardless** (one API) and an **aggregator** (employer per
  posting, stays in the source facet) — same shape as `usajobs`.
- Enumerate the IT slice by searching a **curated list of IT/tech keywords**
  (software, developer, devops, data engineer, cloud, …; the noisy bare "IT" is
  excluded), unioning results and **deduping by Reed `jobId`** across keywords.
- The search list omits `externalUrl` and truncates the description, so the
  adapter **streams a detail fetch per unique job** (`FetchStream` +
  `fetchDetailsStream`, like `eightfold`): `/jobs/{id}` yields the full body and
  the employer's real `externalUrl` (with the Reed page `jobUrl` as fallback).
- Add the boardless placeholder `sources/reed.yml`.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `source-ingest`: add the requirement that a keyed, keyword-scoped aggregator
  source (Reed) is supported — registered only when its API key is configured, and
  enumerating a topical slice via a curated keyword set rather than a board id.

## Impact

- **Code:** new `internal/sources/reed.go` + `reed_test.go` (+ testdata fixtures);
  one line in `sources.All` (env-gated); new `sources/reed.yml` placeholder.
- **No DB / API / web changes.** Jobs flow through the existing pipeline
  (normalize → dedup on `(source, external_id)` → upsert → enrich).
- **Ops (separate, freehire-ops):** `REED_API_KEY` in the host env and a cron
  entry for the reed board file. Not in this repo.
- **Volume:** ~10–15k IT postings on the first crawl; detail-per-job fan-out under
  modest concurrency, leaning on the shared client's 429/5xx retry.
