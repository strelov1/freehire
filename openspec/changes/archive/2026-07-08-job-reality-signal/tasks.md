## 1. Repost fingerprint (schema + write path)

- [x] 1.1 Add migration: `jobs.role_fingerprint text` + btree index on `(company_slug, role_fingerprint)` (backs both counts — repost history any-status and concurrent open); note dev volume recreate + prod manual-apply-before-deploy in the migration comment.
- [x] 1.2 Add a narrow fingerprint function (`jobhash.RoleFingerprint` over `company_slug` + normalized title + normalized description, **excluding** `posted_at`/url/slug); unit tests: bumped `posted_at` → same fingerprint, differing title/description → different fingerprint.
- [x] 1.3 Wire the fingerprint into the write path: added `role_fingerprint` to `UpsertJob` (`jobs.sql` + `make sqlc`), set it beside `content_hash` in `cmd/ingest/store.go` and both `cmd/tg-extract/store.go` sites (NOT `content_hash`).
- [x] 1.4 Integration test (`//go:build integration`): reposts under distinct `external_id`s cluster to one fingerprint; `RoleClusterCount` reports repost/mass counts; NULL/empty fingerprints excluded. Green on real Postgres.

## 2. Reality classifier (pure `internal/jobreality`)

- [x] 2.1 Curated evergreen-text dictionary (EN + RU surface forms: "always hiring", "talent community", "talent pool", RU "всегда в поиске"/"кадровый резерв", …) + phrase matcher; tests: known phrase matches, unmatched text (incl. generic "pipeline") emits nothing (never guesses).
- [x] 2.2 Pure classifier: input `{now, createdAt, postedAt, hasPostedAt, repostCount, massPostingCount, evergreenText}` → `{Class, Evidence{ageDays, repostCount, massPostingCount, fakeFreshness}}`. Rule: `fresh` if age ≤ freshWindow and no evergreen signal; `likely-evergreen` only when ≥2 of {old-age, historical-repost≥k, massPosting≥m, evergreenText} converge (repost signal made independent of mass); else `stale`.
- [x] 2.3 Classifier tests covering every spec scenario: fresh, stale, age-alone-is-not-evergreen, convergence-is-evergreen, mass-posting-alone-is-not-evergreen, fake-freshness recorded when `postedAt` recent but `createdAt` old, determinism.

## 3. Derive, serve, and index

- [x] 3.1 sqlc queries `RoleClusterCount` (single cluster: repost + mass) and `RoleClusterCountsAll` (whole-catalogue GROUP BY, HAVING>1, for the reindex lookup map); both exclude NULL/empty fingerprints. Verified by integration test.
- [x] 3.2 `reality` computed at index time and attached via promoted `doc.Reality` (FromJob signature unchanged — less churn): `jobview.ClassifyReality(job, now, repost, mass)`. Reindex (`reindexFull` + incremental `reindexAll`) precomputes the count map once (`buildRealityLookup`); ingest incremental push computes the single count. Unit test `jobview.ClassifyReality`.
- [x] 3.3 `handler.GetJob` (single Postgres row) computes `reality` from the row + one `RoleClusterCount`, attaches `view.Reality`; count-failure degrades to unique role. (List `/jobs` gets the badge via the Meili search path, not per-row PG.)
- [x] 3.4 `internal/search`: `reality.class` added to `filterableAttributes` + `StringFacets["reality"]→"reality.class"` (query param → facet). New attribute → reindex before it filters (500 window noted).

## 4. Web surface

- [x] 4.1 Regenerated TS contracts (`make gen-contracts`); added `reality.go` to the jobview tygo `IncludeFiles` so the `Reality` interface + `reality?` field emit into `contracts.ts`.
- [x] 4.2 `RealityBadge.svelte` + pure `realityBadge()` helper (vitest): `fresh`→nothing, `stale`→muted age chip, `likely-evergreen`→amber "Likely evergreen" with facts (title + inline `detailed`). Wired into `JobRow` (card) and `JobView` (detail).
- [x] 4.3 Added `reality` facet to `FACETS` (pills, `excludable: true` → opt-in "exclude Likely evergreen"; not hidden by default). Values fresh/stale/likely-evergreen.

## 5. Verify and reconcile

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` green (56 pkg); gofmt clean; DB integration tests green on real Postgres; web `vitest` (64) + `svelte-check` (0 errors) green.
- [x] 5.2 Verification-before-completion: end-to-end HTTP test (`job_reality_integration_test.go`) drives `GET /api/v1/jobs/:slug` on a real Postgres — an old+evergreen-text job serves `reality.class = likely-evergreen` (age_days ~240), a new plain job serves `fresh`. Ship order: apply migration → deploy → backfill `role_fingerprint` → `make reindex` (no `backfill-derive`; reality is index-time).
