## 1. Sitemap enumeration

- [x] 1.1 Write a failing test: given a fixture flat-urlset sitemap mixing canonical
      `/jobs/<slug>-<id>` URLs with unrelated pages, enumeration returns only the
      canonical job URLs.
- [x] 1.2 Implement the job-URL extractor + `sitemapJobLocs` wiring.
- [x] 1.3 Write a failing test: a broken sitemap fetch makes `Fetch` return an error.
- [x] 1.4 Implement that error path.

## 2. Per-posting detail parsing

- [x] 2.1 Write a failing test: a fixture detail page with a `JobPosting` ld+json block
      maps to a `Job` with Title/Description/Location/PostedAt/ExternalID, `Company`
      always `"Airswift"`.
- [x] 2.2 Implement the posting struct and `detail` function (happy path).
- [x] 2.3 Write a failing test: a fixture detail page rendering the
      `c-jobs-article-expired` marker (no `JobPosting` block) is OMITTED from the
      result, not marked `Unreadable`.
- [x] 2.4 Implement the expired-marker detection (walk the parsed tree for a `class`
      attribute containing `c-jobs-article-expired`) and the drop.
- [x] 2.5 Write a failing test: a fixture detail page with neither the expired marker
      nor a `JobPosting` block yields an `unreadableDetail` stub.
- [x] 2.6 Implement that fallback (ensure it's ordered after the expired check).
- [x] 2.7 Write a failing test: a detail fetch returning 404/410 drops the posting
      (no stub); any other fetch error yields `unreadableDetail`.
- [x] 2.8 Implement that `detailUnreadable` branch.

## 3. Markers and registration

- [x] 3.1 Add `boardless()` marker method.
- [x] 3.2 Register `airswift` in `internal/ingest/sources/registry.go`'s `All()`.

## 4. Quality pass and review

- [x] 4.1 Run `simplify` over the new/changed files.
- [x] 4.2 Re-run `go test ./internal/ingest/sources/...` and confirm green.
- [x] 4.3 Request code review; address Critical/Important feedback.

## 5. Post-merge (manual, not part of the tests)

- [ ] 5.1 After merge and deploy, run a manual crawl and sanity-check the job count
      (~750-850 live postings expected).
- [ ] 5.2 Add the live board: `cmd/add-board --provider=airswift --company='Airswift'
      --apply` on the prod host.
