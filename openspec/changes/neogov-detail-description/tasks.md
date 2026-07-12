## 1. Detail extraction

- [x] 1.1 Add a `firstByID` helper in `internal/sources/html.go` (mirroring
  `firstByClass`), with a unit test.
- [x] 1.2 Add `neogovDetailDescription(fragment string) string` in `neogov.go`: parse
  the detail HTML, locate `#details-info`, and return `sanitizeHTML(innerHTML(node))`;
  return `""` when the container is absent or empty. Cover with a `neogov_test.go`
  fixture (full body present, and container missing → empty).

## 2. Wire detail fetch into Fetch

- [x] 2.1 After the listing parse, fetch each card's detail via
  `GetTextWithHeaders(ctx, url, nil)` under bounded concurrency and set
  `Job.Description` to the extracted full body; on fetch error or empty result, keep
  the listing snippet. Update the existing listing test's fake HTTP to serve detail
  pages and assert the full body is stored, snippet is the fallback.
- [x] 2.2 Bound the detail fan-out by reusing the shared, already-tested
  `fetchDetails(_, defaultDetailWorkers, _)` helper (the same bound every other
  detail adapter uses) rather than re-testing the helper's own concurrency guarantee.

## 3. Verify & document

- [x] 3.1 `go build ./... && go vet ./... && go test ./internal/sources/`.
- [x] 3.2 Ran the adapter live against `governmentjobs.com/louisiana` and
  `schooljobs.com/cochisecollege`: full `<dl>` HTML bodies (4–8 KB) now flow on both
  tenants; the `#details-info` selector holds on both. A transient detail-fetch
  failure under concurrency degrades to the listing snippet (confirmed
  non-deterministic across two runs — the same posting self-heals to full HTML on
  re-crawl), never a blank or dropped job. Large boards (louisiana ≈ 644 postings)
  incur the documented N+1 detail-fetch cost, bounded by `defaultDetailWorkers` +
  `board_health` cooldown. No backfill script: re-ingest overwrites snippets in place.
