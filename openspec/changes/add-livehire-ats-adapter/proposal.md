## Why

A visitor submitted a posting hosted on LiveHire (`www.livehire.com/careers/flybuys/job/…`),
a talent platform freehire's catalogue does not crawl at all today: no `atsboard` recognizer, no
`internal/ingest/sources` adapter. LiveHire's HTML career pages sit behind a Cloudflare challenge,
but its JSON API under `/careers-api` does not: the job-search listing (POST) and the per-posting
detail (GET) both answer to a plain client — verified live on 2026-11-03 against the `flybuys`
tenant (6 postings listed, detail returning the full HTML description). That makes it a
straightforward boarded-ATS addition alongside greenhouse/ashby/lever.

## What Changes

- Add a new `livehire` ingest source adapter (`internal/ingest/sources`) that, given a board id
  (the tenant's career-site segment, e.g. `flybuys`), lists that tenant's postings via
  `POST /careers-api/talent-job-search` and hydrates each unseen posting with its full detail
  (HTML description, structured work type, flexible-work option, salary bounds, close date) via
  `GET /careers-api/job/<board>/<urlCode>/<advertUrlCode>`.
- Add an `atsboard` recognizer so `https://www.livehire.com/careers/<segment>` (board listing,
  `/jobs`, and `/job/<urlCode>/<advertUrlCode>/<slug>` posting shapes) resolves to
  `(livehire, <segment>)`; every other path on the vendor's shared product host is declined.
- Register `livehire` in the provider registry so it appears in the source facet and the
  generated `SOURCE_VALUES`.

## Capabilities

### New Capabilities

- `livehire-source`: crawling LiveHire-hosted career sites into the job catalogue via LiveHire's
  public `/careers-api` JSON API (segment-scoped listing, pagination with cross-page dedup,
  per-posting detail hydration, posting normalization).

### Modified Capabilities

(none — `board-harvest`'s existing requirements already cover "one more recognized platform";
only its recognizer registry gains an entry, no requirement changes)

## Impact

- `internal/ingest/sources/livehire.go` (new adapter) + one registration line in
  `internal/ingest/sources/registry.go`.
- `internal/ingest/atsboard/board.go` (new recognizer row + `keyedSegmentBoards` entry).
- Generated source-facet values (`contracts.ts` / `SOURCE_VALUES`) regenerated, not hand-edited.
- No schema migration expected — LiveHire fits the existing `jobs`/`boards` shapes.
