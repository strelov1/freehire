## 1. Adapter

- [x] 1.1 Write `internal/sources/traffit_test.go` (RED): a fake JSON getter returns a
  two-page list (`count` > page size) and the adapter must return all postings mapped
  correctly — title, sanitized description, location from the `geolocation` JSON string,
  `ExternalID` = advert id, `PostedAt` from `validStart`. Include a case with
  `geolocation: null` (empty location, no error) and a posting missing an id (skipped).
- [x] 1.2 Implement `internal/sources/traffit.go` (GREEN): `NewTraffit`, `Provider()`
  → `"traffit"`, `Fetch` paging `?limit=100&offset=N` until `count` collected / empty
  page (with a defensive page cap), mapping items to `Job`. Reuse `sanitizeHTML` and
  `parseEpochSeconds`.
- [x] 1.3 Register `NewTraffit(c)` in `sources.All` (`internal/sources/source.go`).
- [x] 1.4 REFACTOR + simplify the adapter diff; re-run `go test ./internal/sources/`.

## 2. Harvest prober

- [x] 2.1 Write `cmd/harvest-boards/traffit_test.go` (RED): probing a slug whose getter
  returns list JSON with postings reports the count; a slug whose getter errors / returns
  no items yields `("", 0, nil)`. Assert `probers["traffit"]` is registered.
- [x] 2.2 Implement `cmd/harvest-boards/traffit.go` (GREEN): `traffitProber.probe`
  `GetJSON`s the list endpoint, returns `(slug, count, nil)` for a live tenant and a skip
  otherwise; add `"traffit"` to the `probers` map.
- [x] 2.3 REFACTOR + simplify; re-run `go test ./cmd/harvest-boards/`.

## 3. Seed file

- [x] 3.1 Add `sources/traffit.yml` with the validated starter tenants (cloudfide,
  traffit, trust, people, spline, zen, welove, jit, balticamadeus, fintalent, b2bnetwork,
  soflab, itlt, hrhub) — each `company` + `board` (subdomain), with curated display names.
- [x] 3.2 Validate the file loads and passes registry validation:
  `go run ./cmd/ingest sources/traffit.yml` against a local DB (or dry-load), confirming
  no "unknown provider" / empty-board errors.

## 4. Finish

- [x] 4.1 `go build ./... && go vet ./... && go test ./...` all green.
- [x] 4.2 Request code review on the full diff; address Critical/Important.
- [x] 4.3 Verify end-to-end: crawl one tenant and confirm postings persist / a large
  tenant returns > one page of jobs.
