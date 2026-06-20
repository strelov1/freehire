## 1. Avito adapter

- [x] 1.1 Capture live fixtures: the sitemap index, the vacancy sub-sitemap, and two vacancy pages (one remote "Удалённая работа", one on-site city) for table-driven tests
- [x] 1.2 Sitemap enumeration: traverse the index, fetch each sub-sitemap, keep `/vacancies/<cat>/<id>/` locs, dedup by numeric id (test-first)
- [x] 1.3 Detail mapping from `JobPosting` ld+json: `external_id` from URL id, `url`, `title`, sanitized `description`, `posted_at`, `company` from config (test-first)
- [x] 1.4 Location + remote: parse `<title>` "в городе <city>" suffix as location with `addressLocality` fallback; `Remote = isRemote(location)||isRemote(title)` (test-first, covers remote and on-site cases)
- [x] 1.5 Error semantics: failed sitemap-index fetch returns a board error; a failed detail or a page with no `JobPosting` skips just that vacancy (test-first)
- [x] 1.6 `Provider()` returns `"avito"` and the adapter implements the `boardless` marker (test-first)

## 2. Registration & config

- [x] 2.1 Register `"avito": NewAvito(client)` in `sources.All` (`internal/sources/source.go`)
- [x] 2.2 Add one `company: Avito` / `provider: avito` entry to `sources/custom.yml` and confirm `cmd/ingest` validation accepts it

## 3. Verification

- [x] 3.1 `go build ./... && go vet ./... && go test ./internal/sources/` all green
- [x] 3.2 Live smoke: run the adapter against `career.avito.com`, confirm vacancies enumerate, descriptions populate, and at least one remote role is classified remote
