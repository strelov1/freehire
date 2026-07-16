## Why

Djinni (djinni.co) is a major Ukrainian/CEE IT job board (~7,300 open postings) whose
entire guest-visible corpus is served as structured JSON-LD, with full descriptions on the
listing itself. A spike VALIDATED that the listing pages embed a JSON-LD array of complete
`JobPosting` objects, pagination works for anonymous users, ids are stable, and there is no
anti-bot wall. Adding it widens coverage of the CEE market at the cost of a single adapter.

## What Changes

- Add a `djinni` source adapter that crawls the guest listing `https://djinni.co/jobs/?page=N`
  and maps each page's embedded JSON-LD `JobPosting` array to normalized `Job`s (title,
  description, company, employment type, country, posted-at, numeric `identifier` as
  `ExternalID`, canonical detail URL).
- The adapter is **boardless** (one site, no per-tenant board) and an **aggregator** (many
  companies, stays in the source facet, takes each posting's company from the feed).
- Enroll `djinni` in `sources.All` and — via the `aggregator` marker — in the existing
  reindex suppression pass, so a Djinni posting that duplicates a first-party ATS posting
  (same `company_slug` + normalized title + compatible country) is suppressed automatically.
  No new dedup mechanism is introduced; Djinni inherits `aggregator-ats-dedup` and the
  fuzzy/subset title variants.
- Add `sources/djinni.yml` with a single entry (`company: djinni`, `board: all`) crawled by
  its own `cmd/ingest` cron schedule.

## Capabilities

### New Capabilities
- `djinni-source`: the `djinni` adapter — its listing crawl, JSON-LD `JobPosting` mapping,
  pagination and end-of-feed stop, aggregator/boardless classification, and the drop rules
  for unusable postings.

### Modified Capabilities
<!-- None. ATS-dedup is inherited unchanged from aggregator-ats-dedup via the aggregator
     marker + sources.AggregatorProviders(); no spec-level behavior of that capability
     changes. -->

## Impact

- **New code:** `internal/sources/djinni.go` (+ `_test.go`); `sources/djinni.yml`.
- **Touched code:** one line in `sources.All` (registry); the provider becomes visible to
  `sources.AggregatorProviders()` and thus the reindex suppression pass automatically.
- **Ops:** a new `cmd/ingest sources/djinni.yml` cron schedule (deploy-time, in freehire-ops).
- **No migrations, no API changes, no new dependencies.**
