## Why

Australia and New Zealand are absent from the catalogue at any meaningful scale: no per-employer
ATS board covers them, and the region's hiring runs through SEEK, the dominant job board in both
markets (~6.5k live ICT postings in AU, ~1.3k in NZ). Issue #1634 flagged SEEK as the highest-value
target of its batch but could not confirm it was reachable — a plain fetch of a SEEK search page
answers 403. A feasibility spike settled that question: the 403 is a Cloudflare interstitial on the
HUMAN-facing pages only. SEEK's own frontend search API answers 200 JSON to an unauthenticated,
un-spoofed request, and its GraphQL endpoint serves the full posting body. The blocker the issue
raised does not exist on the path we would actually crawl.

## What Changes

- New `seek` source adapter (`internal/sources/seek.go`) crawling SEEK's `/api/jobsearch/v5/search`
  listing and hydrating descriptions from its `/graphql` `jobDetails` operation.
- New board file `sources/seek.yml`: one entry per ICT **subclassification** id (the `board`) per
  **market** (the `region`, `au` or `nz`) — 22 Australian and 21 New Zealand slices.
- The adapter registers as an `aggregator` (many employers, employer read per posting) that is NOT
  boardless, joining `hh` and `whatjobs` in the board-is-a-slice shape.
- It implements `HydratingSource`: a detail request is spent only on a posting the catalogue does
  not already hold, so a steady-state crawl costs listing requests plus one GraphQL call per genuinely
  new posting.
- It declares a 14-day `sweepGrace`, because SEEK stops serving results past roughly the 550th and
  the busiest slices have a tail no crawl can reach.
- No credential, no proxy egress, and no browser-shaped headers: the endpoints are keyless and
  indifferent to User-Agent.

## Capabilities

### New Capabilities
- `seek-source`: crawling SEEK's AU and NZ catalogues — market and slice selection, the listing
  walk and its result-window cap, description hydration, employer resolution, and the sweep-grace
  contract that keeps the unreachable tail from churning.

### Modified Capabilities
<!-- None: this adds a provider behind the existing Source/HydratingSource contracts and changes no
     existing requirement. -->

## Impact

- **New:** `internal/sources/seek.go`, `internal/sources/seek_test.go`, `sources/seek.yml`.
- **Modified:** `internal/sources/registry.go` (one registration line),
  `internal/sources/AGENTS.md` (the platform's verified traps).
- **Ops:** one new ingest cron entry for `sources/seek.yml`, as every board file has.
- **Dependencies:** none — the adapter uses the existing shared HTTP client's `JSONGetter` and
  `JSONPoster` roles.
- **Risk:** both endpoints are frontend internals rather than a documented API, so a SEEK frontend
  change can move either. Board health surfaces that as a failing board rather than silent drift.
