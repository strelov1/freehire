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
- [ ] 4.2 Run it against prod read-only and confirm it reproduces the hand measurement
  that motivated it: the same shape of list, with the software titles the dictionary
  now places absent from it.
- [ ] 4.3 PR, CI green, merge.
