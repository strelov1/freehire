## 1. Fixtures

- [x] 1.1 Save a real `/post/<uuid>` page and a minimal sitemap as test fixtures
  under `internal/sources/testdata/` (trim the post HTML to the flight rows needed
  for parsing), plus confirm whether non-`open` `job_status` appears in the sitemap
  — DONE: `micro1_post.html`, `micro1_sitemap.xml`; 30-post sample all `open`, so a
  `status != open` skip is a cheap defensive guard, not a hard requirement

## 2. Shared RSC helper + detail parsing (TDD)

- [x] 2.1 Promote deel's `deelTextRows`/`deelTextRowMarker` into `nextflight.go` as
  shared `nextFlightTextRows`/`nextFlightTextRowMarker`; repoint deel's call site;
  keep deel tests green
- [x] 2.2 Parse the RSC-flight `data` object from a post page (via `decodeNextFlight`
  + `bracketSlice(flight, "data":, …)`) into a typed struct (`client_job_id`,
  `job_role_name`, `required_skills`, `location_type`, `location_name`, `job_status`,
  `create_datetime`, `job_description` ref); resolve the `$N` description via the
  shared `nextFlightTextRows`
- [x] 2.3 Extract the canonical `/post/<uuid>` id and reject the board root /
  non-post URLs

## 3. Adapter (TDD)

- [x] 3.1 Implement `micro1` adapter: `Provider()`, `boardless()`, and `detail()`
  mapping the parsed payload to a `Job` (ExternalID=client_job_id, Title,
  sanitized Description, Skills, Location/Remote/WorkMode, PostedAt); skip when
  no payload or no `client_job_id`
- [x] 3.2 Implement `Fetch`: GetXML sitemap → filter `/post/<uuid>` → `fetchDetails`
  fan-out; skip non-`open` postings if 1.1 shows they occur

## 4. Wiring

- [x] 4.1 Register `micro1` in the source registry (`internal/sources/source.go`)
- [x] 4.2 Add `sources/micro1.yml` with one boardless `company: micro1` entry

## 5. Verify

- [x] 5.1 `go build ./... && go vet ./... && go test ./internal/sources/`
- [x] 5.2 Dry-run enumerate against live sitemap to confirm real crawl shape
