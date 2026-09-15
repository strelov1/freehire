## 1. Fixtures

- [x] 1.1 Capture a fresh copy of `https://jobs.techtree.dev/sitemap.xml` into
      `internal/ingest/sources/testdata/techtree_sitemap.xml`.
- [x] 1.2 Capture at least two real job detail pages' full HTML into
      `internal/ingest/sources/testdata/techtree_job_*.html` (e.g. the posting already
      sampled during research, plus one more from the sitemap) — real bytes, not
      hand-written, so the `ld+json` block and the `prose`-class description container match
      what the live site actually emits.
- [x] 1.3 Add a malformed/truncated sitemap fixture and a job-page fixture with no
      `application/ld+json` block (or one missing `hiringOrganization`), for the "fails
      loudly" / "dropped" scenarios in the spec. (Implemented as inline HTML string
      constants — `techtreeNoLDJSON`, `techtreeNoHiringOrg` — plus an unrouted sitemap URL
      for the "cannot be fetched" half; a genuinely malformed-XML-body case exercises only
      the shared `getSitemap`/`GetXML` decode path already covered elsewhere, not anything
      techtree-specific, so it was not duplicated here.)
- [x] 1.4 Add a job-page fixture whose `ld+json` block is present and valid but whose page
      carries no `prose`-class container, for the "full body missing → dropped" scenario.
      (`techtreeNoProseBody` inline constant.)

## 2. Sitemap enumeration

- [x] 2.1 Write a failing test asserting `techtreeJobID` matches a `/job/<uuid>` path and
      rejects `/`, `/talent-scout`, `/terms-of-service`, and a job URL with a `?tp=...` query
      string (must still match — the query string is not part of the id check).
- [x] 2.2 Implement `techtreeJobID` (regex over the URL path).
- [x] 2.3 Write a failing test asserting `Fetch` calls `sitemapJobLocs` against
      `https://jobs.techtree.dev/sitemap.xml` and returns exactly the fixture's job URLs
      (1.1), excluding the static-page entries.
- [x] 2.4 Write a failing test asserting a malformed/unreadable sitemap (1.3) makes `Fetch`
      return an error, not an empty success.
- [x] 2.5 Implement the sitemap step of `Fetch` (wrapping `sitemapJobLocs`'s error with
      adapter context, per the `gulftalent`/`thehub` shape).

## 3. Detail page parsing

- [x] 3.1 Define `techtreePosting` (title, `hiringOrganization.name`, `jobLocation.address`
      as a plain string field, `datePosted`, `employmentType`) — deliberately not reusing
      `schemaAddress`/`schemaPlace`, since TechTree's `jobLocation.address` is a bare string.
- [x] 3.2 Write a failing test asserting `ldJobPosting` decodes a normal fixture (1.2) into
      `techtreePosting` with the expected title/company/location/employmentType/datePosted.
      (Verified indirectly through the end-to-end `Fetch` test, which asserts every mapped
      field.)
- [x] 3.3 Write a failing test asserting the fixture with no `ld+json` block (1.3) or no
      `hiringOrganization` name fails the detail parse (posting dropped, not mapped with an
      empty company).
- [x] 3.4 Write a failing test asserting the full rich-text description is read from the
      page's `prose`-class container (via `firstByClass` + `innerHTML`) on the normal fixture,
      and that it is NOT equal to the `ld+json` block's own short `description` field (guards
      against silently regressing to the summary).
- [x] 3.5 Write a failing test asserting the fixture with no `prose`-class container (1.4)
      fails the detail parse (posting dropped).
- [x] 3.6 Implement the detail-parse step: `ldJobPosting` decode, company-presence check, DOM
      description extraction + `sanitizeHTML`, all composed into one `detail(ctx, loc) (Job,
      bool)` method following the `thehub.go`/`gulftalent.go` shape.

## 4. `sources.Job` mapping

- [x] 4.1 Write a failing test asserting a well-formed detail maps to a `sources.Job` with
      `ExternalID` from `techtreeJobID(loc)`, `URL` as the sitemap's own loc (canonical, no
      `tp` param), `Title`/`Company`/`Location` from the decoded posting, `Description` as the
      sanitized full body, `Remote` from `isRemote(location)`, `EmploymentType` from
      `schemaEmploymentType`, and `PostedAt`. (`PostedAt` uses a new `techtreePostedAt`
      wrapper, not `parseRFC3339` directly — discovered live that TechTree's `datePosted` is
      a zone-less ISO timestamp `parseRFC3339` alone does not accept; see the function's
      doc comment in `techtree.go`.)
- [x] 4.2 Implement that mapping (covered by 3.6's `detail` method).

## 5. Adapter wiring

- [x] 5.1 Implement `Provider() string` returning `"techtree"`.
- [x] 5.2 Implement `Fetch(ctx, _ CompanyEntry) ([]Job, error)` composing sitemap enumeration
      (2.x) and `fetchDetails(locs, defaultDetailWorkers, ...)` over the per-posting detail
      parse (3.x/4.x), matching `thehub.Fetch`'s structure.
- [x] 5.3 Register `techtree` with the `aggregator` and `boardless` marker methods (no
      `HydratingSource`/`fullBoardListing` — every posting is hydrated on every crawl, per
      design.md).
- [x] 5.4 Add `techtree_test.go` covering `Fetch` end-to-end against the fixtures (HTTP layer
      stubbed via a fake `techtreeHTTP`), asserting the full set of scenarios from
      `specs/techtree-source/spec.md`.

## 6. Registry

- [x] 6.1 Add `NewTechTree(c)` to `sources.All` in `internal/ingest/sources/registry.go`,
      alongside the other boardless multi-company aggregators (`NewTheHub(c)`,
      `NewCompleo(c)`, `NewInstaffo(c)`).

## 7. Verification

- [x] 7.1 `gofmt -l .` clean on every touched file.
- [x] 7.2 `go vet ./...` and `go test ./...` green. (Whole-module `go test ./...` surfaced
      one pre-existing failure, `TestTheStoreProviderAloneKeepsTheWorkerRunning` in
      `cmd/billing-sync` — unrelated to this change's files, not touched or fixed here.)
- [x] 7.3 Re-read `specs/techtree-source/spec.md` scenario by scenario and confirm each has a
      corresponding test from sections 2-5.
- [x] 7.4 Run the `simplify` pass over the diff. (One improvement: the two
      "posting dropped" tests now include a second, valid posting in the same crawl so they
      isolate "just this one is dropped" from the separate "everything failed" guard, rather
      than relying on both collapsing to the same single-posting outcome.)
