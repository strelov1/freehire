## 1. Recognizer

- [x] 1.1 Add host row `{"jobs.dover.com", "dover", modePathPattern}` with a doc comment
      carrying the live-verification facts (2026-11-05, re-verified in review round 1: the
      link uuid is a POSTING id; the slug is the shared board identity).
- [x] 1.2 Add the `pathPatternBoards` entry: capture the slug ONLY, gated on a REQUIRED
      UUID-shaped second segment, optional trailing posting slug not captured, canonical
      collapses to the `/<Org>` board address.
- [x] 1.3 Positive test cases: the submitted Deepspatial link resolves to
      `(dover, Deepspatial)`; a posting link with a trailing slug resolves to the same board
      and canonical.
- [x] 1.4 Negative test cases: bare `/<Org>` (marketing shell served for any slug), the
      vendor's `/startup-jobs` listing, and a second segment that is not a UUID are all
      declined.

## 2. Adapter and link-source resolver

- [x] 2.1 No ingest-adapter change in the final design: boards are slugs, so the existing
      `careers-page-slug` resolve path is the supported one (an interim uuid-shortcut was
      removed in review round 1 — the link uuid is a posting id, not the client id).
- [x] 2.2 Test: a `jobs.dover.com`-spelled board crawls through the slug lookup and yields
      correct `/apply/<slug>/<jobId>` URLs (`TestDoverFetchNewJobsDoverBoardCrawlsThroughSlug`,
      fixture = live Deepspatial resolution).
- [x] 2.3 Add `internal/ingest/linksource/dover.go`: direct per-job resolution off the
      detail endpoint (client_name naming, slug-namespaced external id), registered in
      `linksource.All` — the review-round-2 findings (shell-provenance reward risk,
      unknown-employer conversion) are answered by resolving the vacancy directly instead of
      writing an unverified board row.
- [x] 2.4 Tests: client-record naming + slug-namespaced id against live fixtures; Match
      declines the shell, `/startup-jobs`, and a non-UUID second segment.

## 3. Validation

- [x] 3.1 `go test ./internal/ingest/atsboard/... ./internal/ingest/sources/...` green.
- [x] 3.2 `go vet` and `gofmt -w` clean.
- [x] 3.3 Live re-check of the recognizer decision table against the submitted link
      (slug resolves, its client lists the posting; the link uuid as a client 404s).
