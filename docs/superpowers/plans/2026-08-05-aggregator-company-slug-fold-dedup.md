# Aggregator/ATS company_slug fold dedup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `SuppressAggregatorDuplicatesForCompany` suppress an aggregator posting against
its ATS twin even when the two sources' raw company names slugify to different strings that
agree once word-break hyphens are removed (`"cfoinsights"` vs `"cfo-insights"`).

**Architecture:** Two independent, additive changes to the existing per-company suppression
pass — no schema-breaking change, no new Go driver logic. (1) The `ats`/`agg` CTE filters in
`internal/db/queries/jobs.sql` compare `replace(company_slug, '-', '')` instead of the raw
column, widening which ATS/aggregator rows are considered for the same company. (2) A new
partial expression index keeps that comparison index-backed instead of falling back to a seq
scan per company in the reindex loop.

**Tech Stack:** PostgreSQL, sqlc (v1.31.1, generates `internal/db/*.go` from
`internal/db/queries/*.sql`), Go integration tests (`-tags=integration`, testcontainers).

## Global Constraints

- English only: code, comments, commit messages, migration text.
- Never hand-edit generated `internal/db/*.go` files — regenerate with sqlc and commit
  whatever it produces.
- Legal-suffix spelling variance (`"Cfoinsights Inc"` vs `"Cfoinsights"`) is explicitly out of
  scope for this change (design doc: `docs/superpowers/specs/2026-08-05-aggregator-company-slug-fold-dedup-design.md`).
- `go vet -tags=integration ./...` must pass before this work is pushed (repo convention,
  `AGENTS.md`) — it is the cheap guard that catches signature drift the plain `go build` and
  `go test ./...` commands miss.
- New index migrations on `jobs` follow the existing repo convention (`migrations/0013`,
  `0023`, `0026`, `0027`, `0033`): the tracked migration file uses blocking
  `CREATE INDEX IF NOT EXISTS` (safe on a fresh/empty volume); a `CONCURRENTLY` build on an
  existing prod volume is a documented, out-of-band operator step, not code in this plan.

---

### Task 1: Fold the company_slug comparison in the suppression query

**Files:**
- Modify: `internal/db/queries/jobs.sql:562-573` (doc comment), `:586` (the `ats` CTE filter),
  `:602` (the `agg` CTE filter)
- Modify (generated, do not hand-edit): `internal/db/jobs.sql.go`
- Test: `internal/db/aggregator_dedup_integration_test.go`

**Interfaces:**
- Consumes: `Queries.SuppressAggregatorDuplicatesForCompany(ctx, SuppressAggregatorDuplicatesForCompanyParams{Company, Aggregators})` — unchanged signature, only the query body changes.
- Produces: the same method, now suppressing across `company_slug` values that fold to the
  same string. No new exported symbol.

- [ ] **Step 1: Write the failing test**

Add this test to `internal/db/aggregator_dedup_integration_test.go` (after
`TestSuppressAggregator_MarksAggregatorDuplicateOfATS`, following the same helper pattern as
`TestCompaniesWithAggregatorPostings_OnlyAggregatorCompanies`, which already overrides
`.Company`/`.CompanySlug` on the params struct returned by `atsJob`/`aggJob`):

```go
func TestSuppressAggregator_FoldsCompanySlugWordSeparatorVariance(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// Two sources spell the same employer differently: greenhouse's raw company name has a
	// space ("CFO Insights" -> "cfo-insights"), arbeitnow's has none ("Cfoinsights" ->
	// "cfoinsights"). company_slug is normalize.Slug(name), which never strips legal
	// suffixes (internal/normalize/slug.go), so the only difference here is where the
	// word-break hyphen landed.
	ats := atsJob("cfoinsights:6500265003", "Founder Associate (MBA Graduate)", []string{"GB"})
	ats.Company = "CFO Insights"
	ats.CompanySlug = "cfo-insights"
	mustUpsert(t, q, ats)

	agg := aggJob("arbeitnow:founder-associate-349135", "Founder Associate (MBA Graduate)", []string{"GB"})
	agg.Company = "Cfoinsights"
	agg.CompanySlug = "cfoinsights"
	mustUpsert(t, q, agg)

	suppressAggregators(t, q)

	atsID, atsDup := dupOf(t, pool, "cfoinsights:6500265003")
	if atsDup != -1 {
		t.Errorf("ATS row duplicate_of = %d, want NULL (canonical)", atsDup)
	}
	if _, aggDup := dupOf(t, pool, "arbeitnow:founder-associate-349135"); aggDup != atsID {
		t.Errorf("aggregator duplicate_of = %d, want ATS %d (company_slug word-separator variance must fold)", aggDup, atsID)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -tags=integration ./internal/db/ -run TestSuppressAggregator_FoldsCompanySlugWordSeparatorVariance -v`

Expected: FAIL — `aggDup` is `-1` (unsuppressed), not the ATS row's id, because the current
query requires exact `company_slug` equality and `"cfoinsights"` (the aggregator's slug,
which is also what `CompaniesWithAggregatorPostings` returns and what the driver passes as
`Company`) never equals `"cfo-insights"` (the ATS row's slug).

Needs Docker running locally (testcontainers spins up Postgres for `startPostgres`).

- [ ] **Step 3: Update the doc comment and both CTE filters**

In `internal/db/queries/jobs.sql`, change the doc comment above the query (currently ending
`... Run AFTER RecomputeRoleDuplicatesForCompany so ATS reposts have already collapsed to
their canon.`) by appending one sentence:

```sql
-- name: SuppressAggregatorDuplicatesForCompany :execrows
-- The per-company slice of the cross-source aggregator suppression. An open aggregator
-- posting is marked duplicate_of an open CANONICAL ATS (non-aggregator) posting of the
-- same company, equal normalized title, and compatible country (countries overlap, or
-- either side empty — the geography dictionary is sparse, so an unresolved side must not
-- veto). The ATS row is never touched, so it stays canonical. Candidate aggregator rows
-- are those that are canonical OR already point at a non-aggregator row (i.e. suppressed
-- by THIS pass) — an aggregator repost pointed at another aggregator by the role pass is
-- left alone. A candidate with no ATS twin resolves to NULL, so a closed twin releases
-- its aggregator copy back into search/embedding/enrichment. min(id) picks a stable
-- target; the IS DISTINCT FROM guard makes re-runs cheap and idempotent. Run AFTER
-- RecomputeRoleDuplicatesForCompany so ATS reposts have already collapsed to their canon.
-- Company match folds away word-separator spelling variance between sources: company_slug
-- is normalize.Slug(name), which never strips legal suffixes, so two sources naming the
-- same employer with a different word break ("Cfoinsights" vs "CFO Insights") land on
-- different slugs ("cfoinsights" vs "cfo-insights") that agree once hyphens are removed.
```

Then change the `ats` CTE filter (currently at line 586):

```sql
    WHERE jobs.company_slug = sqlc.arg(company)
```
to
```sql
    WHERE replace(jobs.company_slug, '-', '') = replace(sqlc.arg(company)::text, '-', '')
```

And the `agg` CTE filter (currently at line 602):

```sql
    WHERE a.company_slug = sqlc.arg(company)
```
to
```sql
    WHERE replace(a.company_slug, '-', '') = replace(sqlc.arg(company)::text, '-', '')
```

Leave every other line in both CTEs (the `closed_at`/`duplicate_of`/`source` conditions)
untouched.

- [ ] **Step 4: Regenerate sqlc**

Run: `make sqlc` (uses Docker). If Docker cannot run sqlc's own container in this
environment, install the pinned version and run it directly instead:
`go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 && $(go env GOPATH)/bin/sqlc generate`
(confirm the version matches the `// versions: sqlc v1.31.1` header already in
`internal/db/jobs.sql.go` before installing).

Then: `git diff --stat internal/db/` — expect only `internal/db/jobs.sql.go` to have changed
(the embedded query string), not `internal/db/querier.go` or the
`SuppressAggregatorDuplicatesForCompanyParams` struct, since no parameter was added or
renamed.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test -tags=integration ./internal/db/ -run TestSuppressAggregator_FoldsCompanySlugWordSeparatorVariance -v`

Expected: PASS.

- [ ] **Step 6: Run the full aggregator-suppression test file to check for regressions**

Run: `go test -tags=integration ./internal/db/ -run TestSuppressAggregator -v` and
`go test -tags=integration ./internal/db/ -run TestCompaniesWithAggregatorPostings -v`

Expected: PASS — all of `TestSuppressAggregator_MarksAggregatorDuplicateOfATS`,
`_EmptyCountryStillMatches`, `_DifferentCountryNotSuppressed`, `_NeverDemotesATS`,
`_TwoAggregatorsWithoutATSUntouched`, `_ReleasesWhenATSTwinCloses`, and
`TestSuppressedAggregator_HiddenFromListAndEnrichmentButServedBySlug` still pass unchanged —
the fold is a superset of exact equality (a slug with no hyphens folds to itself), so none of
these single-company-slug scenarios can regress.

- [ ] **Step 7: Commit**

```bash
git add internal/db/queries/jobs.sql internal/db/jobs.sql.go internal/db/aggregator_dedup_integration_test.go
git commit -m "fix(db): fold company_slug word-separator variance in aggregator suppression

Two sources spelling the same employer differently (\"Cfoinsights\" vs \"CFO
Insights\") produced different company_slug values (\"cfoinsights\" vs
\"cfo-insights\"), and SuppressAggregatorDuplicatesForCompany required exact
equality, so the aggregator copy was never compared against its ATS twin."
```

---

### Task 2: Add the backing index and finish verification

**Files:**
- Create: `migrations/0074_jobs_open_company_slug_folded_idx.sql`

**Interfaces:**
- Consumes: nothing from Task 1's Go/SQL code directly — this is a standalone schema change
  that makes Task 1's new predicate index-backed instead of a per-company seq scan.
- Produces: index `jobs_open_company_slug_folded_idx`, checked by name in Step 3 below.

- [ ] **Step 1: Check the next free migration number**

Run: `ls migrations/ | grep -oE '^[0-9]+' | sort -n | tail -3` to confirm `0074` is still free
in this checkout (it was, as of writing this plan, with `0073` the latest). If another
migration has landed on `main` in the meantime, renumber this file to the next free number —
see the `hire-migration-numbers-are-duplicated` project note: numbers have collided before,
so re-check against `main`, not just this branch, before merging.

- [ ] **Step 2: Write the migration file**

Create `migrations/0074_jobs_open_company_slug_folded_idx.sql`:

```sql
-- Partial expression index backing SuppressAggregatorDuplicatesForCompany's cross-source
-- company match (internal/db/queries/jobs.sql). company_slug is normalize.Slug(name) — plain
-- transliteration and hyphenation, no legal-suffix stripping (internal/normalize/slug.go) —
-- so two sources spelling the same employer with a different word break ("Cfoinsights" vs
-- "CFO Insights") produce different slugs ("cfoinsights" vs "cfo-insights"). The suppression
-- pass now compares replace(company_slug, '-', ''); without this index that predicate
-- seq-scans the whole jobs table once per company in the reindex loop.
--
-- Applied to a fresh volume by initdb after 0073; on an existing prod volume build it
-- CONCURRENTLY out of band (a plain CREATE INDEX would lock the live jobs table):
--   CREATE INDEX CONCURRENTLY jobs_open_company_slug_folded_idx
--     ON public.jobs (replace(company_slug, '-', ''))
--     WHERE closed_at IS NULL AND company_slug <> '';
CREATE INDEX IF NOT EXISTS jobs_open_company_slug_folded_idx
    ON public.jobs (replace(company_slug, '-', ''))
    WHERE closed_at IS NULL AND company_slug <> '';
```

- [ ] **Step 3: Apply it locally and confirm it exists**

Run: `make up` (starts app + Postgres in Docker, if not already running), then
`make migrate` to apply every migration file — including the new one — to the local
database.

Then verify with `make psql`, running:

```sql
SELECT indexdef FROM pg_indexes WHERE indexname = 'jobs_open_company_slug_folded_idx';
```

Expected: one row, `indexdef` containing
`ON jobs USING btree (replace(company_slug, '-'::text, ''::text)) WHERE ((closed_at IS NULL) AND (company_slug <> ''::text))`
(Postgres's own formatting of the same expression). Exit psql with `\q`.

- [ ] **Step 4: Full verification suite**

Run, from the repo root:

```bash
go build ./...
go vet ./...
go vet -tags=integration ./...
go test ./...
go test -tags=integration ./internal/db/
```

Expected: all pass with no errors. `go vet -tags=integration ./...` is the guard this repo's
`AGENTS.md` requires before every push — it compiles all 152 integration-tagged test files
across 13 packages, not just `internal/db`, catching any signature drift this change might
have caused elsewhere (none is expected, since no exported signature changed).

- [ ] **Step 5: Commit**

```bash
git add migrations/0074_jobs_open_company_slug_folded_idx.sql
git commit -m "feat(db): add partial expression index for the folded company_slug dedup match

Backs the replace(company_slug, '-', '') comparison SuppressAggregatorDuplicatesForCompany
now runs per company; without it that predicate falls back to a seq scan of jobs."
```

**Before this ships to prod:** per the migration file's own comment, an operator must build
`jobs_open_company_slug_folded_idx` `CONCURRENTLY` on the prod volume before `cmd/migrate`
reaches this file there (mirrors the existing `migrations/0013` runbook) — a plain
`CREATE INDEX IF NOT EXISTS` on prod's `jobs` table (millions of rows) would hold a lock long
enough to be user-visible. If a staging environment with prod-shaped data is available,
`EXPLAIN (ANALYZE, BUFFERS)` the modified `SuppressAggregatorDuplicatesForCompany` query there
first to confirm the planner actually picks an Index Scan on the new index rather than a seq
scan — local verification in Step 3 only confirms the index exists and is well-formed, not
that the planner prefers it at prod's row counts and selectivity.

---

## Self-Review Notes

- **Spec coverage:** the design doc's two Decisions (folded SQL comparison; new partial
  index, `IF NOT EXISTS` convention) map to Task 1 and Task 2 respectively. The design's
  Non-Goals (legal-suffix folding, `company_slug`'s stored value, cluster-copies key) are
  untouched by both tasks — confirmed no task modifies `internal/jobderive/jobderive.go`,
  `internal/normalize/*.go`, or `ListRoleClusterCopies`.
- **Placeholder scan:** no TBD/TODO; every step has literal, runnable commands or code.
- **Type consistency:** `SuppressAggregatorDuplicatesForCompanyParams{Company, Aggregators}`
  is referenced identically in Task 1 (existing test helper `suppressAggregators`) and is not
  touched by Task 2 — no signature drift between tasks.
