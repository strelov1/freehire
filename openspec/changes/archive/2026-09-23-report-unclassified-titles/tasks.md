## 1. The pure function

- [x] 1.1 Add `dictgap.UnclassifiedTitles` in `internal/job/dictgap/unclassified.go`:
  takes distinct titles with posting counts, returns the ones BOTH `classify.IsTech`
  and `classify.Parse` fail to place, ranked by count then title. Test first: a title
  the tech dictionary places is excluded; a title only the category dictionary places
  is excluded; ranking is by count with a deterministic tie-break; counts for the same
  title arriving in two chunks are summed.

## 2. The reads

- [x] 2.1 Add `UnclassifiedTitleReportBounds` and `ListTitlesForUnclassifiedReport` to
  `internal/platform/db/queries/jobs.sql`, following the ClassifyDrift pair: id-span,
  then per-chunk `GROUP BY title` with a count, scoped to open, canonical, non-private
  postings. No row `LIMIT` — `GROUP BY` bounds the output and a limit on an unordered
  aggregate drops titles silently. Run `make sqlc`.

## 3. The binary

- [x] 3.1 Add `cmd/report-unclassified-titles`, modelled on `cmd/report-classify-drift`:
  walk the id span in chunks, merge counts by title, print the ranked list. Read-only —
  no `--apply`, no write, no timer. Test first: the merge across chunks and the output
  ordering.

## 4. Ship

- [x] 4.1 Full gate: `gofmt -l .` silent, `go vet ./...`, `go test ./...`,
  `go vet -tags=integration ./...`, `golangci-lint --new-from-rev=origin/main`.
- [x] 4.2 Run it against prod read-only and confirm it reproduces the hand measurement
  that motivated it: the same shape of list, with the software titles the dictionary
  now places absent from it.
  **Result (2026-09-23):** 2,653,315 distinct titles walked, **1,581,391 placed by
  neither dictionary**. The top of the list reproduces the hand measurement title for
  title — `Музыкальный руководитель` 1933, the German retail apprenticeships, `Швея`
  1502 — and **none** of `Senior Developer`, `Lead Developer`, `Power Platform
  Developer`, `Flutter Developer`, `SQL Developer`, `Mulesoft Developer`, `IT Officer`
  or `Software Engr` appears anywhere in it, which is the recompute doing its job:
  those postings still read `is_tech NULL` in the database, because the backfill has
  not reached them.
  The run also settled the chunk default — see 4.4.
- [x] 4.3 PR, CI green, merge.
- [x] 4.4 Raise `defaultChunkSize` from the drift report's 50,000 to 2,000,000, on the
  first run's own measurement: the id sequence is far sparser than the row count (12.7M
  rows over a max id of 1.62 billion), so at 50,000 the walk is 32,414 mostly-empty
  chunks and projects to ~3.5 hours, nearly all of it the courtesy pause. At 2,000,000
  the same report finished in 25 minutes with ~15k rows per chunk.
