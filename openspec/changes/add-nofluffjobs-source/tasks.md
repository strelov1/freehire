## 1. Listing crawl & mapping

- [x] 1.1 Add `internal/sources/nofluffjobs_test.go` with an inline listing fixture (`{"postings":[…]}`
  with `id`, `url`, `name`, `title`, `posted`, `technology`, `seniority`, `fullyRemote`,
  `location.places[]`) and a RED test asserting `Fetch` (list-only) maps each posting to a `Job` with
  the mapped fields (id→ExternalID, job/<url> URL, title, company, location/remote, posted→PostedAt,
  technology→Skills, seniority→Seniority) and drops postings missing id/url/company.
- [x] 1.2 Create `internal/sources/nofluffjobs.go`: the `nofluffjobs` struct over a client interface
  combining `GetStream` (listing) and `GetJSON` (detail); the `nofluffjobsPosting` list struct; a
  `crawl` that streams+decodes `{postings:[…]}`; a `toJob` mapper (with `nofluffjobsSeniority` via
  `enrich.SeniorityValues`, skills via `skilltag.Parse`, epoch-ms posted-at, place/remote location);
  and `Fetch`. Make 1.1 green.

## 2. Hydrating FetchNew + detail description

- [x] 2.1 RED test: `FetchNew` with a fake seen-predicate fetches the detail only for new postings
  (assert the exact set of detail slugs requested), sets the description from
  `details.description`+`requirements.description`, leaves seen postings list-only (no detail request),
  and falls back to the list-only job when a detail request errors.
- [x] 2.2 Implement `FetchNew(ctx, entry, seen)`, the `nofluffjobsDetail` struct, `detail(slug)`, and
  the description assembly + `apply`. Make 2.1 green.

## 3. Classification & registration

- [x] 3.1 Add the `boardless()` and `aggregator()` markers and `Provider()` returning `"nofluffjobs"`;
  register `nofluffjobs` in `sources.All` (one line, under the multi-company aggregators). Test the
  provider resolves from `All(nil)` and is in `FilterableProviders()`. Verify `go build ./... && go vet ./...`.

## 4. Board file & docs

- [x] 4.1 Add `sources/nofluffjobs.yml` with a single boardless entry (`company: NoFluffJobs`) and
  confirm `go run ./cmd/ingest sources/nofluffjobs.yml` validates against the registry (fail-fast passes).
- [x] 4.2 No-op: `internal/sources/AGENTS.md` does not enumerate adapters by class.

## 5. Verify end-to-end

- [x] 5.1 Run `go test ./internal/sources/...` (all green), then a throwaway live smoke crawl against
  the real API (a bounded sample) to confirm the listing streams, postings map with facets, and a new
  posting's detail description hydrates without error (not committed).
