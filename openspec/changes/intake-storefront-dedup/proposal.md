## Why

A link pasted from a **storefront** — a careers site on a company's own domain fronting an ATS
board we already crawl — is imported a second time under `(weblink, <the URL>)`, beside the row
the crawl already wrote. Prod holds three such pairs out of the nine `weblink` rows ever written.

The board-level defences do not cover this. An id-in-the-URL lookup exists only for greenhouse
and ashby, so a storefront over teamtailor, lever, workable or anything else still duplicates.
Two of the three pairs in prod are exactly that. Writing one lookup per ATS does not scale —
each needs its own id format and catalogue query — while a single check on what the page
actually parsed to covers every ATS at once.

## What Changes

- Before writing a posting under the generic `weblink` identity, the import asks the catalogue
  for the open canonical posting of the same role cluster (`company_slug` + `role_fingerprint`).
- On a hit the row is still written, but marked `duplicate_of` that canon, kept out of the
  enrichment queue, and not pushed to search. Writing rather than skipping is deliberate: the
  row is what makes the storefront URL resolvable, since URL resolution answers with the posting
  a duplicate duplicates.
- The intake answers `found` with the CANONICAL posting's slug — a second route to that status,
  reached after the import rather than before it.
- The contribution is recorded before that answer is given: the board behind an unrecognised
  storefront may still be new and worth onboarding.
- Board identities are untouched. `UpsertJob` already dedups them on `(source, external_id)`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `posting-import-by-url`: a page whose vacancy the catalogue already carries under another
  source is collapsed onto that posting instead of being imported as a second one, and is
  answered `found` rather than `imported`.

## Impact

- `internal/db/queries/jobs.sql` — two new queries (`CanonicalJobForRole`,
  `MarkJobDuplicateOf`); regenerates `internal/db/`.
- `internal/linkimport/linkimport.go` — the dedup check in `write`; `Result` gains a flag.
- `internal/handler/intake.go` — the second route to `found`.
- `POST /api/v1/jobs/resolve` — no new status; `found` becomes reachable for a link that was
  fetched. Docs (`web/src/lib/docs/api-spec.ts`, generated `docs/API.md`) restate it.
- No migration: the `(company_slug, role_fingerprint)` index already exists.
