## 1. iCIMS adapter

- [x] 1.1 Implement the `icims` `Source` adapter in `internal/sources/icims.go`
  (transport `icimsHTTP = XMLGetter + HTMLGetter`; `Fetch` reads
  `careers-{board}.icims.com/sitemap.xml`, filters `/jobs/{id}/.../job` URLs,
  bounded `fetchDetails` over `?in_iframe=1` fragments parsed with
  `ldJobPosting`; map title/url/location/description/id/posted_at/remote per
  design), driven by `internal/sources/icims_test.go` written first (RED): a stub
  `icimsHTTP` with sitemap-XML + iframe-HTML fixtures asserting id/title/location/
  description/date extraction, `/jobs/search` filtering, `UNAVAILABLE` dropping,
  and one-failing-detail isolation.
- [x] 1.2 Register `NewICIMS(c)` in `sources.All` (`internal/sources/source.go`).

## 2. Harvest prober

- [x] 2.1 Widen the `cmd/harvest-boards` prober `httpClient` interface with
  `sources.XMLGetter` and add `icimsProber` (sitemap lists ≥1 `/jobs/{id}/` URL →
  `(slug, n, nil)`, else skip), driven by `cmd/harvest-boards/prober_test.go`
  cases written first (RED): live sitemap → `n > 0`; empty/404 → skip.
- [x] 2.2 Register `"icims": icimsProber{}` in the `probers` map
  (`cmd/harvest-boards/prober.go`).

## 3. Verify

- [x] 3.1 `go build ./... && go vet ./... && go test ./...` all green.
