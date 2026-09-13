## 1. CSV parsing and validation

- [x] 1.1 Parse a `name,slug,url` CSV file into rows, returning a clear error (and writing
      nothing) when the header is missing the `url` column or another required column
- [x] 1.2 Return a clear error (and write nothing) for a structurally malformed CSV (bad
      quoting, wrong column count)

## 2. Recognition and grouping

- [x] 2.1 Resolve each row's `url` via `internal/ingest/atsboard.Recognize`, skipping and
      counting rows it cannot resolve, without stopping the run
- [x] 2.2 Group recognized rows by provider, deduplicating identical `(provider, board)`
      pairs so each appears once per output file

## 3. Seed file output

- [x] 3.1 Write one JSON seed file per provider into the output directory, as an array of
      `{"board", "company"}` objects (company = the row's `name` column)
- [x] 3.2 Confirm a written seed file decodes into `cmd/harvest-boards`'s own seed shape
      without modification (e.g. a test that unmarshals it the same way that tool does)

## 4. CLI wiring and summary

- [x] 4.1 Wire `-in <csv>` / `-out <dir>` flags and run parse → recognize → group → write
      end to end; exit non-zero and write nothing on a structural input failure
- [x] 4.2 Print a per-provider written-board-count summary plus a total unrecognized-row
      count to stdout on a successful run
- [x] 4.3 Add the package-level doc comment on `cmd/seed-from-inventory/main.go` (purpose,
      usage line, non-goals) matching the style of `cmd/harvest-boards` and `cmd/add-board`

## 5. End-to-end coverage

- [x] 5.1 Add an end-to-end test with a small mixed CSV fixture (rows recognized under two
      different providers, one duplicate `(provider, board)` pair, one unrecognized vanity
      URL) asserting the full set of output files and their exact contents, and the
      reported unrecognized count
