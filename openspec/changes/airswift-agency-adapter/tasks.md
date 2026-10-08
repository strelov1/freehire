## 1. Sitemap enumeration

- [ ] 1.1 Write a failing test: given a fixture flat-urlset sitemap mixing canonical
      `/jobs/<slug>-<id>` URLs with unrelated pages, enumeration returns only the
      canonical job URLs.
- [ ] 1.2 Implement the job-URL extractor + `sitemapJobLocs` wiring.
- [ ] 1.3 Write a failing test: a broken sitemap fetch makes `Fetch` return an error.
- [ ] 1.4 Implement that error path.

## 2. Per-posting detail parsing

- [ ] 2.1 Write a failing test: a fixture detail page with a `JobPosting` ld+json block
      maps to a `Job` with Title/Description/Location/PostedAt/ExternalID, `Company`
      always `"Airswift"`.
- [ ] 2.2 Implement the posting struct and `detail` function (happy path).
- [ ] 2.3 Write a failing test: a fixture detail page rendering the
      `c-jobs-article-expired` marker (no `JobPosting` block) is OMITTED from the
      result, not marked `Unreadable`.
- [ ] 2.4 Implement the expired-marker detection (walk the parsed tree for a `class`
      attribute containing `c-jobs-article-expired`) and the drop.
- [ ] 2.5 Write a failing test: a fixture detail page with neither the expired marker
      nor a `JobPosting` block yields an `unreadableDetail` stub.
- [ ] 2.6 Implement that fallback (ensure it's ordered after the expired check).
- [ ] 2.7 Write a failing test: a detail fetch returning 404/410 drops the posting
      (no stub); any other fetch error yields `unreadableDetail`.
- [ ] 2.8 Implement that `detailUnreadable` branch.

## 3. Markers and registration

- [ ] 3.1 Add `boardless()` marker method.
- [ ] 3.2 Register `airswift` in `internal/ingest/sources/registry.go`'s `All()`.

## 4. Quality pass and review

- [ ] 4.1 Run `simplify` over the new/changed files.
- [ ] 4.2 Re-run `go test ./internal/ingest/sources/...` and confirm green.
- [ ] 4.3 Request code review; address Critical/Important feedback.

## 5. Post-merge (manual, not part of the tests)

- [ ] 5.1 After merge and deploy, run a manual crawl and sanity-check the job count
      (~750-850 live postings expected).
- [ ] 5.2 Add the live board: `cmd/add-board --provider=airswift --company='Airswift'
      --apply` on the prod host.
