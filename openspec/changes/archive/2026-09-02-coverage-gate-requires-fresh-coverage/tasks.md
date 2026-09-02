## 1. The query

The whole gate becomes one statement. `last_seen_at` must stay out of every index —
`RefreshUnchangedJob` documents that the column is unindexed on purpose so the hot re-crawl
update stays heap-only.

**Corrected mid-implementation:** this group was planned as "no migration, the index is already
there". It is on PROD, but only because migration 0109 told an operator to build it by hand —
the file never created it, so any volume built from `migrations/` has the column and no index.
Both reviews caught it. 1.4 records it.

- [x] 1.1 Add `CompaniesWithFreshNonAggregatorCoverage :many` to
      `internal/platform/db/queries/jobs.sql`. Takes the folded slugs asked about, the
      aggregator source list, and the `seen_after` cutoff; returns the folded slugs that have a
      qualifying row. Shape it as `unnest(folded_slugs) ... WHERE EXISTS (...)` so a large
      employer short-circuits on its first fresh row rather than aggregating all of them.
      Comment must state why `last_seen_at` carries no index and must not gain one.
- [x] 1.2 `make sqlc`, and commit the regenerated `internal/platform/db` in the same commit —
      the pre-commit hook and the `sqlc` CI job regenerate and diff.
- [x] 1.3 Integration test (`//go:build integration`, `internal/platform/db`): a fresh
      non-aggregator row is returned; the same row aged past the cutoff is NOT; an aggregator
      row is never coverage however fresh; a closed row is never coverage; a slug asked with
      hyphens matches a stored slug written without them and vice versa.
- [x] 1.4 Add `migrations/0122_jobs_open_company_slug_folded_col_idx.sql` creating
      `jobs_open_company_slug_folded_col_idx` CONCURRENTLY (`migrate: no-transaction`),
      `IF NOT EXISTS` so it is a no-op on prod. CONCURRENTLY rather than 0076's
      plain-CREATE-plus-a-comment shape, which is how the index went missing in the first
      place. Guard it with a presence assertion in the integration test: the failure is
      SILENT — every result stays correct while the query seq-scans ~7.4M rows.

## 2. The adapter

`cmd/ingest` already owns the two other Postgres-backed ports the Runner takes
(`newDBStore`, `newBoardHealth`), so the coverage port joins them rather than growing a package.

- [x] 2.1 `cmd/ingest/coverage.go`: `newCoverage(pool)` implementing
      `pipeline.CoverageLookup`. It folds each asked slug, batches, calls the query with
      `time.Now().Add(-coverageFreshness)`, and credits each answer back to every asked slug
      that folds to it — a folded answer can own more than one spelling.
- [x] 2.2 Define `coverageFreshness = 14 * 24 * time.Hour` beside it, with the measurement
      table from design.md in the comment and the reason it is NOT `sources.DefaultSweepGrace`:
      the sweep asks whether a posting is still on its board, the gate asks whether we still
      crawl the employer, and a board goes uncrawled for far longer than a posting goes
      unlisted. Not an env var — see design.md.
- [x] 2.3 Unit test the fold-and-credit mapping with a fake query runner (no database): two
      spellings of one employer both credited from one answer; an answer for a slug nobody
      asked about never appears in the result.
- [x] 2.4 Integration test (`cmd/ingest`, alongside `board_health_integration_test.go`): the
      real query through the real adapter, asserting the freshness cutoff end-to-end.

## 3. Wiring

- [x] 3.1 Replace `coverageLookup(cfg)` in `cmd/ingest/main.go` with `newCoverage(pool)` and
      rewrite its doc comment — the port is no longer Meili-backed and no longer keyed off
      `cfg.MeiliKey`.
- [x] 3.2 **Note the behaviour change and check it:** the gate used to be silently OFF for any
      run without `MEILI_MASTER_KEY`, and is now on for every run that has a database. Confirm
      against `deploy/` which ingest units actually carry the Meili key today, so the change is
      reported as "gate now also applies to runs X, Y" rather than discovered later.
- [x] 3.3 Update the `CoverageLookup` doc contract in `internal/ingest/pipeline/pipeline.go`:
      coverage means RECENT coverage, the implementation owns the window, and the paragraph
      about Meili being unable to compute the fold goes away — with it, the sentence conceding
      that hyphen-mismatched coverage is "missed by design" is no longer true.

## 4. Remove what this orphans

Verified as having no other reader: `chunkStrings` (0 uses outside the file), `foldSlug` (0),
`search.JobDocument.CompanySlugFolded` (2 uses, both in `document.go`). `InStrings`,
`NotInStrings`, `buildFacetResult` and `facetSearcher` are shared and stay.

- [x] 4.1 Delete `internal/search/search/coverage.go` and its test, and
      `Client.NonAggregatorCompanies` with them.
- [x] 4.2 Delete `JobDocument.CompanySlugFolded`, its assignment in `document.go`, and the
      `"company_slug_folded"` entry in the jobs index `FilterableAttributes` in `client.go`.
      Removing a filterable attribute needs no settings-before-binary dance — that hazard is
      one-directional (a binary querying an attribute the live index has not declared). The
      binary stops querying it first; the live index keeps a harmless declaration until the
      next rebuild drops it.
- [x] 4.3 `go build ./... && go vet ./... && go test ./...`, then
      `go vet -tags=integration ./...`.

## 5. Documentation

- [x] 5.1 `internal/ingest/pipeline/AGENTS.md`: rewrite the "exact slug OR folded, in one OR'd
      filter" bullet and the one after it about not crediting unasked slugs — both describe the
      Meili implementation. Replace with the freshness rule, the measurement, and the
      unrecoverable-vs-recoverable asymmetry that chose the window.
- [x] 5.2 `internal/search/search/AGENTS.md` (and the `FilterableAttributes` comment): drop the
      `company_slug_folded` references now that the field is gone.
- [x] 5.3 `docs/agents/job-lifecycle.md`: no change; the separate defect is linked from the
      proposal instead — a board that leaves `sources/` takes its slugs out of the sweep's
      scope forever, which is what creates the stale rows this gate was trusting. Filed as
      #2328.

## 6. Verify on prod

- [x] 6.1 File the follow-up issue for the never-closed rows of departed boards (11,151 slugs
      whose newest non-aggregator row is >30 days unseen). Out of scope here on purpose: this
      change stops those rows suppressing live postings; it does not close them. Filed as
      **#2328**.
- [x] 6.2a Run `cmd/backfill-slug-folded` on prod. Two write paths were rewriting
      `company_slug` without `company_slug_folded` (7.2/7.3 below), so 1.63% of open rows carry
      a STALE fold — measured 2026-09-02, 1,753 of 107,650 in a 2% sample. The gate now matches
      on the folded column alone, so those rows are invisible to it (and were already invisible
      to the aggregator-suppression pass). The worker's chunk UPDATE is `IS DISTINCT FROM`-
      guarded, so this both repairs the stale rows and writes nothing where the fold is already
      right. Under-matching is the recoverable direction, so this is a repair, not a blocker.
- [x] 6.2 After deploy, run `cmd/ingest` on `sources/himalayas.yml` and confirm
      `GET /api/v1/jobs/find?url=https://himalayas.app/companies/pipe/jobs/senior-software-engineer-5097528323`
      returns a `public_slug` instead of `{"data":null}` — the issue's own reproduction.
      **Done 2026-09-02 17:19 UTC** on the scheduled hourly run (ingested=429, failed=0);
      `/find` now answers `senior-software-engineer-pipe-577lw4nz`, and
      `/jobs/search?company_slug=pipe` returns 2 rows instead of 1 — the fintech's Senior
      Software Engineer beside the 2013 trakstar row that was suppressing it.
- [x] 6.3 Read the run's `Stats.ATSCovered` against the previous run's.

      **The prediction in this line was wrong, and the correction matters more than the
      original guess.** Measured across himalayas' hourly runs on 2026-09-02, out of 2,000
      postings offered:

      ```text
      11:16  1434 covered      15:21  1449 covered
      12:25  1443 covered      16:24  1450 covered   <- last run on the old gate
      14:16  1447 covered      17:19  1491 covered   <- first run on the new gate
      ```

      `ATSCovered` ROSE by 41 (+2.8%), it did not fall. Two effects pull against each other and
      the second is the larger: the freshness window removes coverage, but moving from the
      search index to the live table ADDS it. The index lags its rebuild by hours and carries
      none of the rows `cmd/reindex` drops for search quality — uncategorised, body-less, and
      `duplicate_of`-marked non-aggregator postings — all of which this design counts as real
      coverage on purpose (see design.md, "Coverage definition"). The old gate was therefore
      leaking coverage it should have found, and that leak was bigger than the stale coverage
      it was wrongly claiming.

      So the earlier "a fall is the measure of the fix" is not a usable check, and neither is
      "a collapse to near-zero means something is wrong" — that one still holds, but nothing
      approached it. **The measure of the fix is the reproduction in 6.2**, which passed.

## 7. Review findings applied

Two parallel reviews (standards, spec) ran against the implementation commit. What they
changed, so the next reader knows these were not the first draft:

- [x] 7.1 The missing index — both axes flagged it independently. See 1.4.
- [x] 7.2 `InStrings` / `NotInStrings` / `inStrings` in `internal/search/search/filter.go`
      deleted: removing `coverage.go` left them with no caller but their own tests, and
      `golangci-lint`'s `unused` does not catch exported functions.
- [x] 7.3 `TestRunCoverageMatchesExactCompanySlugOnly` renamed to
      `TestRunKeysCoverageByTheSlugItWillStore` and its rationale rewritten. The assertion is
      still right; its stated reason ("a live Meili filter cannot compute the fold") described
      a system that no longer exists, and the new integration test asserts the opposite of what
      the old comment claimed.
- [x] 7.4 Residual "Meili" wording removed from `pipeline.go` and `pipeline_test.go`.
- [x] 7.5 `TestRunCoverageLookupFailureSavesEverything` added: the delta spec's "Coverage
      lookup fails" scenario had a test for the `coverageProbe` path only, not for the batched
      one the scenario is about.
- [x] 7.6 The two integration tests de-duplicated: the case matrix stays at the db layer, and
      `cmd/ingest`'s test now proves only what that layer alone can — that the three
      parameters are wired through. The db test's seeds were hoisted out of its subtests so
      none depends on another's writes.
- [x] 7.7 CodeRabbit: `NOT is_private` added to the coverage query. `cmd/reindex` drops
      private rows from the index, so the search-backed lookup excluded them by accident and
      reading the table directly did not. A private posting is one user's pasted job
      description, crawled from nowhere, and it is written with `last_seen_at = now()` so the
      freshness window would not have caught it either. Pinned by an integration case
      confirmed to fail without the clause.
- [x] 7.8 CodeRabbit: `UpdateManualJob` was rewriting `company_slug` and leaving
      `company_slug_folded` stale — which matters more now that the folded column is the ONLY
      thing the gate matches on.
- [x] 7.9 The rule that exists to prevent exactly 7.8 had a hole, and closing it is the real
      fix. `writesJobsCompanySlug` tested for `"set company_slug ="`, which reads only the
      column immediately after SET; both offenders assign it mid-list. Widened to scan the
      whole SET clause (bounded before WHERE/FROM/RETURNING, so a read is still not a write),
      which immediately found a SECOND offender: **`UpdateJobDerived`**, the statement
      `cmd/backfill-derive` runs over the entire catalogue. Both fixed, the population guard
      raised 5 -> 6, and `TestAssignsCompanySlugInSetClause` added — the detector's own
      failure mode is silence, which is what let this sit.
