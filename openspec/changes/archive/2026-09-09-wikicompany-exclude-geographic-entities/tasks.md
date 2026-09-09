## 1. Fix

- [x] 1.1 Add `nonOrganizationAnchorQIDs` (geographic location, administrative territorial entity) to `internal/job/wikicompany/query.go`.
- [x] 1.2 Extend `buildOrganizationCheckQuery` with `FILTER NOT EXISTS` over the new anchors.
- [x] 1.3 Unit test: the query text asserts both the positive anchors and the `FILTER NOT EXISTS` exclusion with its own anchors.
- [x] 1.4 Confirm live against `query.wikidata.org`: the reported false-positive QID (Q270195) reaches an exclusion anchor; every spike-confirmed company match does not.

## 2. Verification

- [x] 2.1 `gofmt -l .`, `go vet ./...`, `go test ./...` clean.
- [x] 2.2 After deploy, re-run the live diagnostic (`Lookup` against "Nissan", "Paladin Energy", "Hitachi Energy", "Royal Bank of Canada", "CACI") against production and confirm "Nissan" now yields no match while the others are unaffected. Confirmed on production commit `e3e26678`: Nissan → no match, Paladin Energy/Hitachi Energy/Royal Bank of Canada → matched correctly (one transient 502 from query.wikidata.org on a cold query, gone on retry — not a regression, and exactly what `runBackfill`'s count-and-continue handles).
