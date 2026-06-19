## 1. Test fixtures

- [x] 1.1 Capture a trimmed real `/api/frontend/vacancies?type=all&sort=date&page=1` list
  response (a few vacancies spanning `remoteWork: true`/`false`, multi-location, and a missing
  `company`/`publishedDate` edge) and one `vacancies/<id>` detail HTML (with a `JobPosting`
  ld+json description) into `internal/sources/testdata/` for the adapter test. No network in
  tests.

## 2. Shared detail parse extraction

- [x] 2.1 Extract the Habr `JobPosting` ld+json description parse currently inline in
  `internal/linksource/habrcareer.go` into one reusable helper (in `internal/sources`, reusing
  `sources.LDJobPosting` + `sources.SanitizeHTML`), and call it from the linksource adapter
  unchanged. Keep linksource's existing tests green (preserve current behavior).

## 3. Adapter core

- [x] 3.1 Define the listing response/item structs and the `habrCareer` adapter type with
  `Provider()` returning `"habr_career"`, `boardless()`, `aggregator()`, and a constructor over
  a private interface embedding `HeaderJSONGetter` + `HTMLGetter` (mirroring `breezy`).
- [x] 3.2 Implement `Fetch`: request `page=1` of
  `/api/frontend/vacancies?type=all&sort=date` with headers `Accept: application/json` and
  `Referer: https://career.habr.com/vacancies`, read `meta.totalPages`, paginate `page=2..N`
  (max-page guard), stop early on empty `list`; first-page failure errors, later-page failure
  ends enumeration returning jobs so far.
- [x] 3.3 Map each list item to `Job`: `ExternalID`=`id`, `URL`=`https://career.habr.com/vacancies/<id>`,
  `Title`=`title`, `Company`=`company.title`, `Location`=distinct `locations[].title` joined,
  `PostedAt`=`publishedDate.date`; set `Remote` and `WorkMode="remote"` iff `remoteWork` is true.
- [x] 3.4 For each vacancy, GET the detail page `vacancies/<id>` and set `Description` from the
  shared helper (2.1); on missing ld+json or a failed detail request, yield the vacancy with an
  empty description rather than dropping it.

## 4. Registration and config

- [x] 4.1 Register `habr_career` in `sources.All` (one constructor line).
- [x] 4.2 Add a dedicated `sources/habrcareer.yml` with one boardless placeholder entry
  (mirroring `getmatch.yml`/`tecla.yml`: aggregators get their own file + cron slot so the
  per-vacancy detail fan-out crawls independently). A new `cmd/ingest sources/habrcareer.yml`
  cron line is an ops change at deploy time (not in this repo).

## 5. Verification

- [x] 5.1 `go build ./... && go vet ./... && go test ./internal/sources/ ./internal/linksource/`
  all green, and a live smoke run of the adapter against the real API yields ~748 normalized
  jobs with full descriptions and correct dedup identity with the linksource adapter.
