## Why

Jobspresso (jobspresso.co) is a curated remote-jobs board. It publishes a keyless WordPress Job
Manager RSS feed at `/feed/?post_type=job_listing` that carries every listed posting inline —
title, canonical detail URL, company + location, HTML body, and posted date — so one request per
crawl yields the recent window with no per-posting detail fetch. Adding it widens remote coverage
at the cost of a single adapter that reuses the existing RSS + aggregator machinery.

## What Changes

- Add a `jobspresso` source adapter that fetches the WordPress Job Manager RSS feed at
  `https://jobspresso.co/feed/?post_type=job_listing` and maps each `<item>` to a normalized
  `Job`: the numeric `p=` post id from `<guid>` as `ExternalID` (falling back to the link slug),
  the item `<link>` as the canonical URL, the item `<title>` as the role, the company and
  location split out of `<dc:creator>` (format `Company<br>⚲&nbsp;Location`), the inline
  `<content:encoded>` HTML (falling back to `<description>`) as the sanitized body, and
  `<pubDate>` as `PostedAt`.
- The adapter is **boardless** and an **aggregator**: one global feed with no per-tenant board,
  and the company comes from each posting, so it stays in the source facet. Jobspresso lists only
  remote work, so every posting maps to `Remote: true` with work mode `remote`.
- An item with no usable `ExternalID` or an empty company is dropped rather than emitted with an
  empty dedup key / broken slug (mirrors the weworkremotely rule).
- Enroll `jobspresso` in `sources.All` and add `sources/jobspresso.yml` with a single boardless
  entry, crawled by its own `cmd/ingest` cron schedule.

## Capabilities

### New Capabilities
- `jobspresso-source`: the `jobspresso` adapter — its RSS fetch, `<item>` → `Job` mapping,
  `dc:creator` company/location split, `guid` `p=` id extraction with slug fallback, drop rules
  for unusable items, and boardless-aggregator remote-only classification.

### Modified Capabilities
<!-- None. Boardless aggregator adapter; inherits the standard ingest sweep, aggregator dedup,
     and board-health machinery unchanged. No spec-level behavior of an existing capability
     changes. -->

## Impact

- **New code:** `internal/sources/jobspresso.go` (+ `_test.go`); `sources/jobspresso.yml`.
- **Touched code:** one line in `sources.All` (registry).
- **Ops:** a new `cmd/ingest sources/jobspresso.yml` cron schedule (deploy-time, in freehire-ops).
- **No migrations, no API changes, no new dependencies, no proxy (keyless, direct fetch works).**
