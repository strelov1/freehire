## 1. Recognizer

- [x] 1.1 Add host row `{"jobs.dover.com", "dover", modePathPattern}` with a doc comment
      carrying the live-verification facts (2026-11-05).
- [x] 1.2 Add the `pathPatternBoards` entry: capture `/<Org>/<client-uuid>`, optional trailing
      posting slug not captured, canonical collapses to the two-segment board address.
- [x] 1.3 Positive test cases: the submitted Deepspatial link resolves to
      `(dover, Deepspatial/a5546acf-…)`; a posting link with a trailing slug resolves to the
      same board and canonical.
- [x] 1.4 Negative test cases: bare `/<Org>` (marketing shell served for any slug), the
      vendor's `/startup-jobs` listing, and a second segment that is not a UUID are all
      declined.

## 2. Adapter

- [x] 2.1 `resolveClientID` short-circuits a pair board: second segment matching the
      client-UUID shape is returned as the client id, skipping `careers-page-slug`.
- [x] 2.2 Test: a pair board lists directly under the pair's uuid with NO slug-lookup route
      registered — a lookup would fail the crawl instead.
- [x] 2.3 Test: a plain slug board still resolves through `careers-page-slug` unchanged.

## 3. Validation

- [x] 3.1 `go test ./internal/ingest/atsboard/... ./internal/ingest/sources/...` green.
- [x] 3.2 `go vet` and `gofmt -w` clean.
- [x] 3.3 Live re-check of the recognizer decision table against the submitted link (Deepspatial
      board listing returns `count: 4`).
