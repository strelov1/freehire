## Why

Gem (jobs.gem.com) hosts company job boards that several IT-segment companies use, and
its postings are exposed through a public, unauthenticated GraphQL endpoint with no
bot-wall. Adding a `gem` adapter brings those boards into the shared pool with the same
one-line-per-board ergonomics as every other platform.

## What Changes

- Add a `gem` source adapter (`internal/sources/gem.go`) speaking the existing `Source`
  interface, registered with one `NewGem(c)` line in `sources.All`.
- The adapter follows the established **list → detail** pattern (like
  smartrecruiters/rippling/bamboohr): the list operation carries no description, so each
  posting's body is fetched via a bounded-concurrency detail request using the shared
  `fetchDetails` helper.
- Transport is `POST https://jobs.gem.com/api/public/graphql` (public profile, no auth)
  with two operations: `JobBoardList(boardId)` for the postings and
  `ExternalJobPostingQuery(boardId, extId)` for each description. The `sources.yml`
  `board` value is the Gem **vanity path** (e.g. `go-cadre`), passed verbatim as the
  GraphQL `boardId`.
- Add a small `parseEpochSeconds` time helper beside the existing `parseEpochMillis`, since
  Gem dates its postings with `firstPublishedTsSec` (Unix seconds).
- Add at least one `gem` entry to `sources.yml`.

## Capabilities

### New Capabilities
<!-- None. Reuses the source-ingest pipeline and write path unchanged. -->

### Modified Capabilities

- `source-ingest`: add a requirement that `gem` is a registered provider — a GraphQL
  list→detail adapter yielding the normalized job shape with a sanitized-HTML description,
  consistent with the existing detail-fetching adapters.

## Impact

- **New code**: `internal/sources/gem.go` + `internal/sources/gem_test.go`; one registration
  line in `internal/sources/source.go` (`sources.All`); one `parseEpochSeconds` helper.
- **Config**: one new `sources.yml` entry. No new env vars.
- **DB**: none — reuses `UpsertJob` (`source = "gem"`, namespaced `external_id`). No migration.
- **Dependencies**: none — uses the existing shared `HTTPClient.PostJSON` and `sanitizeHTML`.
- **Out of scope (known seam)**: `JobBoardList` takes no pagination arguments; this is fine
  for single-company Gem boards but would truncate a very large board. Revisit pagination
  only if an observed board exceeds one page.
