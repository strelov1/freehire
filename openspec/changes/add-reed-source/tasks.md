## 1. Adapter (TDD)

- [x] 1.1 Capture fixtures from the live API into `internal/sources/testdata/`:
  a search page (`reed_search.json`) and a job detail (`reed_job.json`).
- [x] 1.2 RED: write `internal/sources/reed_test.go` — Provider()=="reed";
  boardless+aggregator markers; registered only when `REED_API_KEY` set
  (mirroring usajobs); board file validates; a fetch test over a fake client that
  asserts (a) jobId dedup across keywords, (b) Basic-auth header carried, (c)
  externalUrl→URL with jobUrl fallback, (d) full description from detail.
- [x] 1.3 GREEN: implement `internal/sources/reed.go` — `reed` struct
  (HeaderJSONGetter + apiKey), `NewReed`, Provider/boardless/aggregator, the
  curated IT keyword list, `FetchStream` (search-paginate per keyword → unique
  jobIds → `fetchDetailsStream` detail) + buffering `Fetch`, date layout
  `02/01/2006`, description sanitized.
- [x] 1.4 Register `reed` in `sources.All`, env-gated on `REED_API_KEY` (next to
  usajobs). Add `sources/reed.yml` placeholder.

## 2. Verify

- [x] 2.1 `go test ./internal/sources/` green; `go build ./... && go vet ./...`.
- [x] 2.2 `gofmt` clean.
- [x] 2.3 Live smoke against the real API with the key: confirm the adapter
  yields IT jobs with real `externalUrl`s and full descriptions (bounded sample).
