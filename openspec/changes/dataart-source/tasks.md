# Tasks

## 1. Adapter helpers (pure)

- [x] 1.1 `dataartVacancyCode(url)` — returns the `{code}` for a canonical English
  `https://www.dataart.team/vacancies/{code}` URL, `""` for the listing root and
  `/xx/vacancies/...` localisations. Table test.
- [x] 1.2 `dataartPosting.location()` — joins `jobLocation` places as
  `"City, Country"` (fallback to whichever part is present), deduped, and strips
  DataArt's internal `Remote.*` region codes. Test.

## 2. Fetch + mapping + registration

- [x] 2.1 `dataart.Fetch` — sitemap enumerate → filter → per-vacancy ld+json
  detail → `Job`, boardless marker, `Provider() == "dataart"`. Routed-HTTP
  fixture test asserting the full mapping (id, title, location, description,
  posted-at, structured `WorkMode`) and that localised/listing URLs are excluded.
- [x] 2.2 Register `NewDataArt(c)` in `sources.All` (boardless single-company
  section).

## 3. Config + verify

- [x] 3.1 Add `sources/dataart.yml` (one boardless `DataArt` entry).
- [x] 3.2 `go build ./...`, `go vet ./...`, `go test ./internal/sources/` green.
- [x] 3.3 Live smoke: the real adapter fetched 136 DataArt jobs mapping correctly
  (title/location/posted-at/remote).
