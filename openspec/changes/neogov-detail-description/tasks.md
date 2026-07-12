## 1. Detail extraction

- [ ] 1.1 Add a `firstByID` helper in `internal/sources/html.go` (mirroring
  `firstByClass`), with a unit test.
- [ ] 1.2 Add `neogovDetailDescription(fragment string) string` in `neogov.go`: parse
  the detail HTML, locate `#details-info`, and return `sanitizeHTML(innerHTML(node))`;
  return `""` when the container is absent or empty. Cover with a `neogov_test.go`
  fixture (full body present, and container missing → empty).

## 2. Wire detail fetch into Fetch

- [ ] 2.1 After the listing parse, fetch each card's detail via
  `GetTextWithHeaders(ctx, url, nil)` under bounded concurrency and set
  `Job.Description` to the extracted full body; on fetch error or empty result, keep
  the listing snippet. Update the existing listing test's fake HTTP to serve detail
  pages and assert the full body is stored, snippet is the fallback.
- [ ] 2.2 Verify the bound: a board with many cards issues at most the configured
  concurrent detail requests (assert via a counting/blocking fake).

## 3. Verify & document

- [ ] 3.1 `go build ./... && go vet ./... && go test ./internal/sources/`.
- [ ] 3.2 Manually run the adapter against the live board behind
  `sources/neogov.yml` for one agency (or a targeted temp board file) and confirm a
  known posting now carries the full description; confirm re-ingest overwrites an
  existing snippet in place (no backfill script needed).
