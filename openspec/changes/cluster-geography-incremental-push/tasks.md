## 1. The per-cluster geography query

- [x] 1.1 Add a failing integration test in `internal/db` for `RoleClusterGeo`: a multi-city,
  multi-country open cluster returns the sorted union of its open rows' countries/regions/
  cities; a closed row does not contribute; a singleton and an unknown key return empty.
- [x] 1.2 Write `RoleClusterGeo :one` in `internal/db/queries/jobs.sql` next to
  `RoleClusterCount`, mirroring `RoleClusterGeoAll`'s LATERAL-unnest shape scoped to one
  `(company_slug, role_fingerprint)`. Run `make sqlc`.

## 2. Stop the ingest write path narrowing the canon

- [x] 2.1 Add a failing integration test in `cmd/ingest`: after a multi-city role collapses to
  one canon, re-saving the canon with changed content pushes a document that still carries the
  cluster's cities — today it carries only the canon's own.
- [x] 2.2 In `cmd/ingest/store.go`, when `mass > 1`, fetch `RoleClusterGeo` and call
  `doc.MergeClusterGeography` before handing the document to the indexer. A lookup failure logs
  and leaves the canon's own geography, matching how the count already degrades.

## 3. The other two incremental writers

- [x] 3.1 Same treatment in `internal/linkimport/linkimport.go`.
- [x] 3.2 Same treatment in `cmd/embed/indexer.go`, inside the existing `!pgOnly` branch — the
  pg-only mode builds no Meili document and must keep skipping both lookups.

## 4. Verify

- [x] 4.1 `go build ./...`, `go vet ./...`, `gofmt -l`, `go test ./...`.
- [x] 4.2 `go test -tags=integration ./internal/db/ ./cmd/ingest/` (Docker required).
- [x] 4.3 Confirm the singleton path issues no second query: the merge sits behind `mass > 1`,
  which RoleClusterCount already answered, so a role with one open posting pays nothing.
