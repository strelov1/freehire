## Why

Issue #2080 groups `gr8people.com` and `workgr8.com` as one ~6-hit entry, flagging as a preflight
question "if they're one vendor on two domains it is one adapter and one `sources/*.yml`" (the
mistake `internal/atsboard/AGENTS.md` warns about — a board already crawled looking brand new
under a second provider name). Live research for this change confirms they are one vendor: the
same "app-career-site" Next.js frontend, byte-identical GraphQL schema, and the same JWT-issuing
mechanism, served under two marketing domains. It is next in the issue's ranked order after
`jobappnetwork` (done) and `dayforcehcm` (done before this repo's tracked history).

## What Changes

- New `internal/ingest/sources` adapter, single provider key `gr8people`, for the ATS behind both
  `*.gr8people.com` and `*.workgr8.com` — one vendor, the Factorial/FactorialHR precedent
  (`internal/ingest/sources/factorial.go`) for "one provider, two domains".
- A board is the tenant's whole careers host (e.g. `etrade.gr8people.com`), matching Factorial's
  and Teamtailor's `modeHost` convention — the brand suffix varies per tenant, so the host is the
  board identity, not a bare subdomain label.
- Fetch mints a short-lived (5h) anonymous JWT by reading it out of the tenant's own `/jobs` page
  (a `"token":"eyJ..."` value embedded in the page's Next.js `__NEXT_DATA__` blob — no cookie, no
  session, no credential beyond that token), then pages the tenant's own `/graphql` endpoint with
  the site's own `searchJobs` (→ `searchJobPostings`) query, Bearer-authorized. The default,
  unfiltered query already returns exactly what a real visitor's search would (open, externally
  visible postings only — confirmed across three tenants on both domains), so unlike
  `jobappnetwork` no extra filter clause is needed. The listing carries each posting's whole
  `descriptionHTML`, so this is not a `HydratingSource`.
- New rows in `internal/ingest/atsboard`'s `atsBoards` table (`modeHost`, provider `gr8people`,
  once for each domain label) so a person-pasted or harvested job/listing URL on either domain
  resolves to the same provider.
- One line in `sources.All` (`registry.go`).
- `internal/ingest/sources/AGENTS.md` gains a "gr8people traps" section recording what was
  verified live (the JWT mechanism, cursor pagination, the two-domain confirmation, structured
  `workplaceType`/`positionType` fields).

Not in scope: `werecruit.io`, the last platform in the issue's shortlist.

## Capabilities

### New Capabilities
- `gr8people-source`: crawling one gr8people/workgr8 employer board (a tenant career-site host)
  through its own GraphQL search API, and recognizing both domains' public URLs as one provider.

### Modified Capabilities
(none)

## Impact

- `internal/ingest/sources/gr8people.go` (+ `_test.go`): new adapter.
- `internal/ingest/sources/registry.go`: one new `registry["gr8people"] = ...` line (or a `reg(...)`
  entry if no special session wiring is needed — confirmed keyless single-request-per-board token
  mint, so it wires like `NewFactorial(c)`).
- `internal/ingest/atsboard/board.go`: two new `atsBoards` rows (`gr8people`, `workgr8`), both
  `modeHost` → provider `gr8people`.
- `internal/ingest/sources/AGENTS.md`: one new "traps" section.
- No schema change, no new env var, no worker change.
