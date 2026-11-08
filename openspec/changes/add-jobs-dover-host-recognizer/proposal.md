## Why

A visitor submitted a posting hosted on `jobs.dover.com/Deepspatial/a5546acf-…` (board-submission
id 605). Dover is already a crawled source (`dover`, from add-dover-ats-adapter), but its
recognizer only covers the product host `app.dover.com` (`/apply/<slug>/<id>`,
`/jobs/<slug>`); the public job-board host `jobs.dover.com` is not recognized, so such links
fall into the manual-review queue even though the company is crawlable today.

The shape was live-verified on 2026-11-05 (and re-verified during review): the host serves
`/<Org>/<uuid>` where `<Org>` is the company's Dover slug — the SAME identity the
`app.dover.com` apply and listing paths use. The uuid in the submitted link is a POSTING id
(`/inbound/application-portal-job/a5546acf-…` returns `client_id: 1aeeba55-…`;
`careers-page/a5546acf-…/jobs` 404s; `careers-page/1aeeba55-…/jobs` lists the company's 4
postings), so the uuid is proof that the link names a board, not the board itself.

## What Changes

- Extend the `dover` recognizer in `internal/ingest/atsboard` with the host `jobs.dover.com`:
  a URL shaped `/<Org>/<uuid>[/<posting-slug>]` resolves to `(dover, <Org>)` — the slug alone,
  exactly the board id an `app.dover.com` link spells — with the canonical collapsing to the
  `/<Org>` board address. The UUID-shaped second segment is REQUIRED (the marketing shell
  serves a bare `/<Org>` for any slug, and the vendor's `/startup-jobs` listing carries no
  tenant), but it is not folded into the board id: it may be the client id or a posting id.
- No adapter changes: a slug board (whether spelled by `app.dover.com` or `jobs.dover.com`)
  resolves through the existing `careers-page-slug` lookup, which doubles as the existence
  check the host's shell cannot provide. Posting URLs keep the correct
  `/apply/<slug>/<jobId>` shape by construction.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `link-contributions`: the Supported-ATS board recognition registry gains the
  `jobs.dover.com` host — a vacancy or board link on it yields `(dover, <Org>)`, the same
  board an `app.dover.com` link for the same company spells.

## Impact

- `internal/ingest/atsboard/board.go`: one host row (`jobs.dover.com`, `dover`,
  `modePathPattern`) + one `pathPatternBoards` entry capturing the slug, gated on a
  UUID-shaped second segment; positive and negative test cases in `board_test.go`.
- No adapter changes (`internal/ingest/sources/dover.go` untouched by the final design; the
  adapter test gains a case pinning that a `jobs.dover.com`-spelled board crawls through the
  slug path with correct apply URLs).
- No new provider key: `dover` is already registered, so generated `SOURCE_VALUES` are
  unchanged. No schema migration.
