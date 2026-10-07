## 1. Sitemap enumeration

- [x] 1.1 Write a failing test: given a fixture sitemap-index XML (the top-level
      `https://www.energyjobline.com/sitemap.xml` shape) and fixture sub-sitemaps, the
      adapter's enumeration step returns every job-detail URL across all sub-sitemaps.
- [x] 1.2 Implement the job-URL extractor (matches the `.../job/<slug>-<id>` shape) and
      wire it through the existing `getSitemap`/sub-sitemap walk so it handles the index
      shape (reuse, do not reimplement).
- [x] 1.3 Write a failing test: a sitemap index that fails to fetch/decode makes `Fetch`
      return an error (not an empty success).
- [x] 1.4 Implement that error path.

## 2. Per-posting detail parsing

- [x] 2.1 Write a failing test: a fixture job-detail page with `WebSite`, `Organization`,
      and `JobPosting` ld+json blocks (in that order, matching the live page) decodes to a
      `Job` with the `JobPosting`'s title/description/location/datePosted — proving
      `ldJobPosting` is actually wired to skip the non-`JobPosting` blocks for this
      adapter's own posting struct.
- [x] 2.2 Implement the posting struct and `detail` function.
- [x] 2.3 Write a failing test: `hiringOrganization.name` present and non-empty becomes
      the job's `Company`.
- [x] 2.4 Implement that resolution.
- [x] 2.5 Write a failing test: `hiringOrganization.name` empty or the `JobPosting` block
      missing entirely both yield an `unreadableDetail` stub (not a dropped posting, not a
      guessed company).
- [x] 2.6 Implement that fallback.
- [x] 2.7 Write a failing test: `ExternalID` comes from the job-detail URL and a second
      fetch of the same URL maps to the same `ExternalID`.
- [x] 2.8 Implement that id extraction.

## 3. Markers, transport, and registration

- [x] 3.1 Add `boardless()` and `aggregator()` marker methods to the adapter type.
- [x] 3.2 Wire the adapter through `fetchDetails`/`defaultDetailWorkers` for the detail
      fan-out (one failing/passing test proving one bad detail page does not fail the
      whole crawl, per spec).
- [x] 3.3 Register `energyjobline` in `internal/ingest/sources/registry.go`'s `All()`
      alongside `dataart` (plain shared client, no fingerprint transport needed).

## 4. Quality pass and review

- [x] 4.1 Run `simplify` over the new/changed files.
- [x] 4.2 Re-run `go test ./internal/ingest/sources/...` and confirm green.
- [ ] 4.3 Request code review (`requesting-code-review`); address Critical/Important
      feedback.

## 5. Post-merge (manual, not part of the tests)

- [ ] 5.1 After merge, run a manual crawl of `energyjobline` and sanity-check the job
      count/duration (see design.md's volume risk).
- [ ] 5.2 Add the live board on the prod host: `cmd/add-board --provider=energyjobline
      --board='www.energyjobline.com' --company='EnergyJobline' --apply`.
