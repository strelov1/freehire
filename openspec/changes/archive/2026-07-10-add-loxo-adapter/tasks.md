## 1. Fixtures

- [x] 1.1 Save real Loxo HTML fixtures under `internal/sources/testdata/loxo/`: a
  listing page and 1-2 detail pages (from the spike agencies, incl. one with a
  title-delimiter client and one plain agency title)

## 2. Loxo adapter — detail parse

- [x] 2.1 RED: test that `loxo` maps a detail fixture to a `Job` — title stripped
  of ` | <agency>`, description from the embedded JSON blob, URL canonical
  `<host>/job/<base64>`, `ExternalID` = decoded `<agency_id>-<slug>`
- [x] 2.2 GREEN: implement the detail parse in `internal/sources/loxo.go`
- [x] 2.3 RED+GREEN: best-effort location/remote from the detail DOM; empty when
  absent (no guessing)

## 3. Loxo adapter — listing crawl

- [x] 3.1 RED: test that a listing fixture yields one `/job/<base64>` link per
  posting and resolves each against the board host (subdomain / bare / pod)
- [x] 3.2 GREEN: implement listing fetch + link extraction + bounded-concurrency
  detail fan-out (drop only failing postings)
- [x] 3.3 RED+GREEN: `Provider()` returns `"loxo"`; board id `<host>/<slug>` splits
  to origin + slug

## 4. Hub employer attribution

- [x] 4.1 RED: test `hub: true` resolves the client on an explicit title delimiter
  (`— Client` / `@ Client`) and falls back to the agency name otherwise
- [x] 4.2 GREEN: implement Hub-aware company resolution

## 5. Registry + board file

- [x] 5.1 Register `loxo` in `internal/sources/source.go` `All`
- [x] 5.2 Add `sources/loxo.yml` seeded with the validated spike agencies, all
  `hub: true`; `go run ./cmd/ingest sources/loxo.yml` validates against the registry

## 6. harvest-loxo prober

- [x] 6.1 RED: test slug/host extraction from footprint URLs (careers + `/job/`
  URLs across host variants) yields distinct `(host, slug)` candidates
- [x] 6.2 GREEN: implement footprint enumeration + candidate extraction
- [x] 6.3 Implement live validation (200 + `/job/` links; skip dead, never abort)
  and per-board tech-count via `internal/classify`
- [x] 6.4 Emit draft `sources/loxo.yml` entries (`company`, `board: <host>/<slug>`,
  `hub: true`), de-duplicated against the existing file

## 7. Verify

- [x] 7.1 `go build ./... && go vet ./... && go test ./...` green
- [x] 7.2 Run `harvest-loxo` against the live footprint and confirm it emits
  validated draft entries; sanity-run `cmd/ingest sources/loxo.yml` end-to-end
