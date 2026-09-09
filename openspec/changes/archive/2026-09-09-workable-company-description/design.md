## Context

See `proposal.md` for the coverage spike (32/40 real production boards, 80%) and the
`details=true` payload-size finding. `internal/ingest/sources/workable.go`'s `Fetch`
already decodes `{"jobs": [...]}` from `apply.workable.com/api/v1/widget/accounts/{board}?details=true`
into a struct that declares only the `Jobs` field — the response also carries a
top-level `name`/`description` today, just undecoded.

## Goals / Non-Goals

**Goals:** Fill `CompanyDescriber` for Workable at a request cost proportionate to
what it returns (a short account description), not proportionate to the board's full
job listing.

**Non-Goals:** Sharing a single request between `Fetch` and `CompanyDescription` (a
cache keyed by board inside the adapter). Considered and rejected — see Decision
below.

## Decisions

**1. `CompanyDescription` omits `details=true` rather than sharing `Fetch`'s request
or caching its result.**

The pipeline calls `CompanyDescription` independently of `Fetch` (currently before
it, per `Runner.ingestBoard` — see `ingest-native-company-description`'s design), so
sharing a single response between the two would need a mutable, concurrency-safe
cache inside the `workable` adapter value, which every provider in the registry
shares across all boards crawled concurrently (`defaultConcurrency = 8`). That is
real complexity — a mutex-guarded map with an eviction policy — to save requests
that, once `details=true` is dropped, are already cheap (~6 KB, confirmed live).
Two independent lightweight requests is simpler and carries no shared-state risk;
revisit only if Workable's board count or response size ever makes the extra
request measurably expensive.

**2. No `html.UnescapeString`, matching `Fetch`'s own posting-description handling.**

`Fetch` already sanitizes job descriptions with `sanitizeHTML(j.Description)` alone
(no unescape) — the Workable widget API serves raw HTML at both the posting and
account level, unlike Greenhouse's job-listing endpoint (which needed unescaping)
versus its board-metadata endpoint (which didn't). `CompanyDescription` mirrors the
existing convention: `sanitizeHTML(resp.Description)`, no unescape.

## Risks / Trade-offs

- **[Risk]** A board with no `description` field costs a request that returns
  nothing. → **[Mitigation]** Same shape as the 20% empty rate already measured;
  the request is cheap (~6 KB) and the fill-gap write simply doesn't happen — no
  different from Greenhouse's own partial fill rate.
- **[Risk]** The 80% figure came from 40 boards; a larger sample could regress it.
  → **[Mitigation]** Not a correctness risk — a lower real-world rate just means a
  smaller yield, not a wrong write. No gate needed before shipping.

## Migration Plan

Ship behind no flag, same as Greenhouse's `CompanyDescriber` — additive, rides the
existing scheduled Workable ingest crawl. Verify after deploy via
`GET /api/v1/companies/{slug}` on a board confirmed filled in the spike (e.g.
`islacare`), the same way Greenhouse's rollout was confirmed.
