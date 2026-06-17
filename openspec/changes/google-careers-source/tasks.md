## 1. Fixture & blob extraction

- [x] 1.1 Capture a real list-page HTML fixture (`testdata/google_list.html`) from
  `…/jobs/results?page=1`, trimmed to a few representative job records, and commit it.
- [x] 1.2 (RED→GREEN) Write `extractDS1` that, given an `*html.Node`, finds the
  `AF_initDataCallback({key:'ds:1', …})` script and returns its `data` JSON array; test it
  against the fixture (asserts it parses and `data[0]` is non-empty, `data[3]` is the total).

## 2. Record mapping

- [x] 2.1 (RED→GREEN) Implement the positional record accessors + `toJob`, mapping a job
  record to the normalized `Job` (external_id, url=`…/jobs/results/<id>` (id only, slug
  optional), title, location from the locations array, sanitized-HTML description from
  description+responsibilities+qualifications, posted_at via `parseEpochSeconds`). Test the
  full mapping of one fixture record field by field, including the url and a nil date
  when the timestamp is zero/absent.

## 3. Adapter & paging

- [x] 3.1 (RED→GREEN) Implement the `google` adapter type, `NewGoogle`, `Provider()`,
  `boardless()`, and `Fetch` paging loop (page 1.. until empty page or total reached) over an
  `HTMLGetter`. Test `Fetch` with a fake client serving a page-1 fixture then an empty page,
  asserting all records are yielded once and the loop stops.
- [x] 3.2 Register `NewGoogle(c)` in `sources.All` (beside uber/amazon) and add one `google`
  entry to a `sources/*.yml` board file.

## 4. Quality & integration

- [x] 4.1 `simplify` pass over the diff, then `go build ./... && go vet ./... && go test
  ./internal/sources/` green.
- [x] 4.2 Live smoke: `go run ./cmd/ingest sources/<file>.yml` against a scratch DB (or a
  one-off `Fetch` harness) confirms real jobs are returned and mapped sanely.
