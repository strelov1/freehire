## 1. Adapter mapping (single tenant)

- [x] 1.1 Add `internal/sources/cleverstaff_test.go` with a saved `getAllOpenVacancy` JSON
  fixture under `testdata/` (one real tenant capture) and a RED test asserting the adapter
  maps each `objects` element to a `Job` with the right fields: `position`→Title, sanitized
  `descr`→Description, `vacancyId`→`ExternalID`, URL `…/i/vacancy-<localId>`, `dc`/`dm`→
  `PostedAt`, `workCondition`→`WorkMode`, `employmentType`→`EmploymentType`.
- [x] 1.2 Create `internal/sources/cleverstaff.go`: a `cleverstaff` struct over the HTTP
  client, a `cleverstaffVacancy` struct for the JSON fields, and a `toJob` mapper that reuses
  the shared `sanitizeHTML`, `workplaceTypeMode`, and `parseEpochMillis` helpers. Add the
  `employmentType`→`EmploymentType` map (fullEmployment→full_time, partEmployment→part_time,
  contract/freelance→contract, internship→internship; unknown→""). Make 1.1 green.
- [x] 1.3 Add the non-ok/error test + code: a payload whose `status` is not `"ok"` (or a
  transport error) makes `Fetch` return an error, not an empty slice.

## 2. Drop rules & status filter

- [x] 2.1 RED test: an object with no `vacancyId`, no `localId`, or no `position` is dropped
  (not yielded), a non-open `status` is filtered out, and one bad object does not abort the
  rest of the tenant.
- [x] 2.2 Implement the per-object guards and the open-status filter in `toJob`/`Fetch`; make
  2.1 green.

## 3. Hub employer resolution

- [x] 3.1 RED test: with `CompanyEntry{Hub: true}` a vacancy's `clientName` becomes
  `Job.Company` (blank `clientName` falls back to the configured company); with `Hub` unset the
  `Job.Company` is always the configured company regardless of `clientName`.
- [x] 3.2 Implement the `e.Hub` branch in `Fetch` mirroring huntflow; make 3.1 green.

## 4. Classification & registration

- [x] 4.1 Test that `cleverstaff` is a plain per-tenant provider: it requires a board (config
  validation rejects an entry with no board), it is NOT in `AggregatorProviders(All(nil))`, and
  NOT in `filterableProviders` exclusions (it stays a normal board provider). Add `Provider()`
  returning `"cleverstaff"`.
- [x] 4.2 Register `cleverstaff` in `sources.All` (one line) and add the `proxiedProviders`
  entry rebuilding it over the proxied client. Verify `go build ./... && go vet ./...`.

## 5. Board file & docs

- [x] 5.1 Harvest a seed set of tenant aliases via search-engine dorking
  (`site:cleverstaff.net inurl:-vacancies`), validate each against
  `getAllOpenVacancy?alias=<x>` (status ok + non-empty objects), and add `sources/cleverstaff.yml`
  with the validated entries (`company` + `board`; `hub: true` for agency tenants). Include
  `doit-software1` (confirmed live). Confirm `go run ./cmd/ingest sources/cleverstaff.yml`
  passes fail-fast validation.
- [x] 5.2 Note the new source in `internal/sources/AGENTS.md` if it enumerates adapters by
  class, keeping the surrounding style (keyless JSON, per-tenant, proxied-provider).

## 6. Verify end-to-end

- [x] 6.1 Run `go test ./internal/sources/...` (all green) and a throwaway live smoke fetch of
  a seeded tenant to confirm real vacancies map without error (not committed). Note the prod-IP
  block risk in the change's handoff so the cron timer stays disabled until a prod smoke or
  `SOURCES_PROXY_URL` confirms egress.
