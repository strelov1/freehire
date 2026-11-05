## Why

A visitor submitted a posting hosted on `jobs.dover.com/Deepspatial/a5546acf-…` (board-submission
id 605). Dover is already a crawled source (`dover`, from add-dover-ats-adapter), but its
recognizer only covers the product host `app.dover.com` (`/apply/<slug>/<id>`,
`/jobs/<slug>`); the public job-board host `jobs.dover.com` addresses a company's board as
`/<Org>/<client-uuid>` and is not recognized, so such links fall into the manual-review queue
even though the board is crawlable today.

The board pair was live-verified on 2026-11-05: `careers-page-slug/Deepspatial` resolves the
company and returns the client UUID `1aeeba55-…`, `careers-page/1aeeba55-…/jobs` lists its 4
published postings, and `inbound/application-portal-job/a5546acf-…` returns the full detail of
the submitted posting (whose `client_id` matches). The two-segment shape `/<Org>/<client-uuid>`
IS the identity the careers API pages under.

## What Changes

- Extend the `dover` recognizer in `internal/ingest/atsboard` with the host `jobs.dover.com`:
  a URL shaped `/<Org>/<client-uuid>[/<posting-slug>]` resolves to
  `(dover, <Org>/<client-uuid>)`, with the canonical collapsing to the two-segment board
  address. Every other path on the host (the bare marketing shell `/<Org>`, the vendor's own
  `/startup-jobs` listing) is declined. This is one host row plus one `pathPatternBoards`
  entry — no adapter changes to reach recognition.
- Teach the existing `dover` adapter to address a pair board directly: a board whose id
  carries `<slug>/<client-uuid>` uses the uuid as the client id for the listing endpoint,
  skipping the `careers-page-slug` lookup (which 404s on a pair, and whose marketing shell
  would resolve any slug anyway).

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `dover-source`: a board may be addressed either by its `app.dover.com` slug (resolve-then-
  list, unchanged) or by the `jobs.dover.com` pair `<Org>/<client-uuid>` (list directly by the
  uuid the pair carries).
- `link-contributions`: the Supported-ATS board recognition registry gains the
  `jobs.dover.com` host — a vacancy or board link on it yields `(dover, <Org>/<client-uuid>)`,
  the same source as `app.dover.com` links.

## Impact

- `internal/ingest/atsboard/board.go`: one host row (`jobs.dover.com`, `dover`,
  `modePathPattern`) + one `pathPatternBoards` entry with the two-segment UUID pattern;
  positive and negative test cases in `board_test.go`.
- `internal/ingest/sources/dover.go`: `resolveClientID` short-circuits a pair board to its
  uuid; test cases in `dover_test.go`.
- No new provider key: `dover` is already registered, so generated `SOURCE_VALUES` are
  unchanged. No schema migration. No changes to the adapter's Job mapping, listing walk, or
  hydration.
