## 1. Detail mapping (single open vacancy)

- [x] 1.1 Add `internal/sources/onstrider_test.go` with a saved real open-vacancy HTML fixture
  under `testdata/` and a RED test asserting the adapter maps its `JobPosting` ld+json to a
  `Job` with the right fields: `identifier.value` (UUID) → `ExternalID`, page URL → `URL`,
  title, sanitized HTML description, company `Strider`, `applicantLocationRequirements[].name`
  countries → `Location`, `jobLocationType: TELECOMMUTE` → `Remote: true` + work mode `remote`,
  `datePosted` → `PostedAt`.
- [x] 1.2 Create `internal/sources/onstrider.go`: an `onstrider` struct over the sitemap+HTML
  client, an `onstriderPosting` struct for the schema.org fields (nested `identifier`
  PropertyValue, `applicantLocationRequirements` array, `employmentType` array), and a
  `detail`/`toJob` mapper reusing `ldJobPosting`, `sanitizeHTML`, `schemaEmploymentType`, and
  `workModeFromRemote`. Make 1.1 green.

## 2. Open/closed drop rules

- [x] 2.1 RED test: a saved closed-vacancy fixture (page with no `JobPosting` block) yields no
  `Job`; a page with a `JobPosting` whose `identifier.value` is empty is dropped; a fetch error
  on one URL does not abort the crawl.
- [x] 2.2 Implement the drop rules in `detail` (return `ok=false` on missing `JobPosting` or
  empty `identifier.value`) and confirm `fetchDetails` skips just that URL. Make 2.1 green.

## 3. Sitemap enumeration & URL filter

- [x] 3.1 RED test: given a sitemap fixture mixing canonical `/jobs/<slug>-<8hex>`, blog,
  marketing, `/pt/`+`/en/` localized, and `/preview-slug-<uuid>/...` URLs, `Fetch` fetches only
  the canonical vacancy URLs (assert the exact set) via a fake sitemap+detail client.
- [x] 3.2 Implement `Fetch`: `GetXML` the sitemap, filter with an anchored
  `onstriderVacancyURL` regex, then `fetchDetails(urls, defaultDetailWorkers, d.detail)`. Make
  3.1 green.

## 4. Classification & registration

- [x] 4.1 Add the `boardless()` marker and `Provider()` returning `"onstrider"`; register
  `onstrider` in `sources.All` (one line). Test the provider resolves from `All(nil)`.
- [x] 4.2 Add `onstrider` to `proxiedProviders` and a test asserting membership (mirrors the
  djinni proxied-provider test). Verify `go build ./... && go vet ./...`.

## 5. Board file & docs

- [x] 5.1 Add `sources/onstrider.yml` with a single boardless entry (`company: Strider`) and
  confirm `go run ./cmd/ingest sources/onstrider.yml` validates against the registry
  (fail-fast passes).
- [x] 5.2 Note the new adapter in `internal/sources/AGENTS.md` if it enumerates adapters by
  class (JSON-LD front / sitemap), keeping the surrounding style.

## 6. Verify end-to-end

- [x] 6.1 Run `go test ./internal/sources/...` (all green), then a throwaway live smoke crawl
  against onstrider.com to confirm the sitemap enumerates, open vacancies map, and closed ones
  are dropped without error (not committed).
