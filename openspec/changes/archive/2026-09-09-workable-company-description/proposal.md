## Why

`internal/ingest/sources`' `CompanyDescriber` capability (shipped for Greenhouse in
`openspec/changes/archive/2026-09-09-ingest-native-company-description`) was
deliberately not implemented for Workable at the time: the design spike sampled
Workable's account-level `description` field on two boards (GitLab, Canva) and found
it empty on both, so implementing it looked like paying for a request with no
payoff. A follow-up spike against 40 real boards drawn at random from the production
`boards` table (not two hand-picked large companies) found the field **filled on 32
of 40 (80%)** — GitLab and Canva were an unrepresentative pair of large companies
that manage their careers presence differently, not evidence about the platform.
Coverage this high, at zero extra ongoing cost (the request already exists in the
adapter contract; only the field is unused), is worth capturing.

## What Changes

- Implement `CompanyDescription` on the Workable adapter (`internal/ingest/sources/workable.go`):
  fetch `apply.workable.com/api/v1/widget/accounts/{board}` — the same endpoint
  `Fetch` calls, but WITHOUT `?details=true` — and return the sanitized `description`
  field. Confirmed live: dropping `details=true` shrinks the response ~11x (65997 →
  5940 bytes on one sampled board) while the top-level `description` field is
  unaffected (`details` only controls whether each individual job's own full body is
  inlined). So this is a second request, but a cheap one — comparable in cost to
  Greenhouse's separate board-metadata request, not a duplicate of the (potentially
  much larger) per-posting-detail payload `Fetch` pays for.
- No change to the `CompanyDescriber` interface, the pipeline wiring, the
  `CompanyDescriptionFiller` Store capability, or the SQL write — all of that already
  exists and needs nothing Workable-specific.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
(none — `source-ingest`'s "an adapter MAY implement CompanyDescriber" requirement
already covers a second adapter implementing it; no requirement text changes)

## Impact

- `internal/ingest/sources/workable.go` only.
- No schema, API, or pipeline change. One additional lightweight (~6 KB) request per
  Workable board per crawl.
