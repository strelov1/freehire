## 1. Test fixtures

- [ ] 1.1 Capture a trimmed real `/api/frontend/vacancies?type=all&sort=date&page=1` list
  response (a few vacancies spanning `remoteWork: true`/`false`, multi-location, and a missing
  `company`/`publishedDate` edge) and one `vacancies/<id>` detail HTML (with a `JobPosting`
  ld+json description) into `internal/sources/testdata/` for the adapter test. No network in
  tests.

## 2. Shared detail parse extraction

- [ ] 2.1 Extract the Habr `JobPosting` ld+json description parse currently inline in
  `internal/linksource/habrcareer.go` into one reusable helper (in `internal/sources`, reusing
  `sources.LDJobPosting` + `sources.SanitizeHTML`), and call it from the linksource adapter
  unchanged. Keep linksource's existing tests green (preserve current behavior).

## 3. Adapter core

- [ ] 3.1 Define the listing response/item structs and the `habrCareer` adapter type with
  `Provider()` returning `"habr_career"`, `boardless()`, `aggregator()`, and a constructor over
  a private interface embedding `HeaderJSONGetter` + `HTMLGetter` (mirroring `breezy`).
- [ ] 3.2 Implement `Fetch`: request `page=1` of
  `/api/frontend/vacancies?type=all&sort=date` with headers `Accept: application/json` and
  `Referer: https://career.habr.com/vacancies`, read `meta.totalPages`, paginate `page=2..N`
  (max-page guard), stop early on empty `list`; first-page failure errors, later-page failure
  ends enumeration returning jobs so far.
- [ ] 3.3 Map each list item to `Job`: `ExternalID`=`id`, `URL`=`https://career.habr.com/vacancies/<id>`,
  `Title`=`title`, `Company`=`company.title`, `Location`=distinct `locations[].title` joined,
  `PostedAt`=`publishedDate.date`; set `Remote` and `WorkMode="remote"` iff `remoteWork` is true.
- [ ] 3.4 For each vacancy, GET the detail page `vacancies/<id>` and set `Description` from the
  shared helper (2.1); on missing ld+json or a failed detail request, yield the vacancy with an
  empty description rather than dropping it.

## 4. Registration and config

- [ ] 4.1 Register `habr_career` in `sources.All` (one constructor line).
- [ ] 4.2 Add one boardless placeholder entry for `habr_career` to `sources/custom.yml`
  (already on the hourly `fh-custom` cron — no cron change needed).

## 5. Verification

- [ ] 5.1 `go build ./... && go vet ./... && go test ./internal/sources/ ./internal/linksource/`
  all green, and a live smoke run of the adapter against the real API yields ~748 normalized
  jobs with full descriptions and correct dedup identity with the linksource adapter.
