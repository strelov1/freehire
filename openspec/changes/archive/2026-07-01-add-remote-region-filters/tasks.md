## 1. International → global (job-geography)

- [x] 1.1 Add a failing test in `internal/location` asserting `Parse("Remote - International")` yields `regions=[global]`, `work_mode=remote`, empty countries (plus a synonym case).
- [x] 1.2 Add `"international"` (+ close worldwide synonyms) → `global` to `nameToRegion` in `internal/location/dictionaries.go`; confirm the `RegionValues`-membership invariant test still passes.

## 2. remote_unspecified derivation (jobderive)

- [x] 2.1 Add a failing test in `internal/jobderive` covering the three cases from the spec: bare remote → true; remote+region (incl. `global`) → false; non-remote/empty-geo → false.
- [x] 2.2 Add `RemoteUnspecified bool` to `jobderive.Derived` and compute it in `Derive` as `work_mode == "remote" && len(Countries) == 0 && len(Regions) == 0`.

## 3. Storage (migration + sqlc)

- [x] 3.1 Add migration `migrations/00XX_remote_unspecified.sql` adding `jobs.remote_unspecified BOOLEAN NOT NULL DEFAULT false`.
- [x] 3.2 Update the relevant queries in `internal/db/queries/*.sql` (UpsertJob and the derive/backfill write path) to set `remote_unspecified`; run `make sqlc` and commit generated code.
- [x] 3.3 Wire the derived value into the pipeline and moderator write paths and `cmd/backfill-derive` so the column is written on ingest and on re-derive.

## 4. Serving (jobview)

- [x] 4.1 Add a failing `internal/jobview` test asserting the served object reports `remote_unspecified` from the column.
- [x] 4.2 Serve `remote_unspecified` from the `jobs` column in `jobview` (dict-only, beside the other facets).

## 5. Search (Meili attribute + filter param)

- [x] 5.1 Add a failing `internal/search` test asserting `FilterFromValues` emits an `EqBool` fragment for `remote_unspecified=true` and nothing when unset.
- [x] 5.2 Register `remote_unspecified` as a filterable attribute in `internal/search/client.go`; populate it in the search document from the read model.
- [x] 5.3 Handle the `remote_unspecified=true` param in `FilterFromValues` via `EqBool` (mirroring `visa_sponsorship`).

## 6. SPA filter pill (web-frontend)

- [x] 6.1 Add a `remote_unspecified` filter control to the Region group in `web/src/lib/facets.ts`, labelled "remote, region not specified" and distinct from `Global`; verify via `svelte-check`.

## 7. Rollout

- [x] 7.1 `go build ./... && go vet ./... && go test ./...` green; `svelte-check` clean.
- [x] 7.2 Document the deploy ordering in the change: run `cmd/backfill-derive` then a full `make reindex` (fresh index + swap) BEFORE the API/UI exposes the new param, to avoid `/jobs` 500s on the unknown filterable attribute.
