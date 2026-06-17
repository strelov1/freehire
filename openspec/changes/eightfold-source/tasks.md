## 1. Board parsing

- [x] 1.1 (RED→GREEN) Implement `parseEightfoldBoard("host/domain")` returning the host and
  domain, erroring on a board missing either half. Test a valid board, a board with no `/`, and
  a board with an empty half.

## 2. Job mapping (list + detail)

- [x] 2.1 (RED→GREEN) Map a list position plus its detail to the normalized `Job` (in the
  `detail` method): `external_id` = numeric id stringified, `url` = `canonicalPositionUrl` else
  `https://<host>/careers/job/<id>`, `title`, `location` (first of `locations`), `work_mode` via
  `workplaceTypeMode(workLocationOption)`, `posted_at` via `parseEpochSeconds(postedTs)`,
  `description` = `sanitizeHTML(job_description)`. Test the full mapping field by field against
  canned JSON, including the URL fallback when `canonicalPositionUrl` is empty and a nil date
  when `postedTs` is zero.

## 3. Adapter, paging & detail fan-out

- [x] 3.1 (RED→GREEN) Implement the `eightfold` adapter type, `NewEightfold`, `Provider()`, and
  `Fetch`: parse the board, page `/api/pcsx/search` by `start` (stop on empty page or running
  count ≥ `data.count`), then fetch each position's detail via `fetchDetails`. Test `Fetch`
  with a fake `JSONGetter` serving a page-1 fixture then an empty page plus per-id detail
  bodies, asserting all positions are yielded once, the loop stops, and a detail-fetch failure
  drops only that one posting.
- [x] 3.2 Register `NewEightfold(c)` in `sources.All` and add the `sources/eightfold.yml` board
  file (Microsoft + Micron on pcsx; Netflix on the legacy list).
- [x] 3.3 (RED→GREEN) Support the legacy `/api/apply/v2/jobs` list generation with auto-detect:
  `listPositions` tries pcsx, then falls back to the v2 list (top-level positions/count,
  `t_create` date, single-string `location`, `work_location_option`). One `eightfoldPosition`
  decodes both field-name variants; the detail endpoint is shared. Test the fallback with a fake
  serving no pcsx route + a v2 list, asserting the v2 fields map correctly.

## 4. Quality & integration

- [x] 4.1 `simplify` pass over the diff, then `go build ./... && go vet ./... && go test
  ./internal/sources/` green; `make gen-contracts` and commit the regenerated
  `web/src/lib/generated/contracts.ts` (adds `eightfold` to `SOURCE_VALUES`).
- [x] 4.2 Live smoke: a one-off `Fetch` against the real Microsoft board confirms real jobs are
  returned and mapped sanely (title, url, location, non-empty description, dated).
