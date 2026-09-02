## 1. The query

The whole gate becomes one statement. No migration and no new index: both indexes it needs are
already on prod (`jobs_open_company_slug_folded_col_idx` on `(company_slug_folded) WHERE
closed_at IS NULL AND company_slug <> ''`, verified 2026-09-02), and `last_seen_at` must stay
out of every index — `RefreshUnchangedJob` documents that the column is unindexed on purpose so
the hot re-crawl update stays heap-only.

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

## 2. The adapter

`cmd/ingest` already owns the two other Postgres-backed ports the Runner takes
(`newDBStore`, `newBoardHealth`), so the coverage port joins them rather than growing a package.

- [ ] 2.1 `cmd/ingest/coverage.go`: `newCoverage(pool)` implementing
      `pipeline.CoverageLookup`. It folds each asked slug, batches, calls the query with
      `time.Now().Add(-coverageFreshness)`, and credits each answer back to every asked slug
      that folds to it — a folded answer can own more than one spelling.
- [ ] 2.2 Define `coverageFreshness = 14 * 24 * time.Hour` beside it, with the measurement
      table from design.md in the comment and the reason it is NOT `sources.DefaultSweepGrace`:
      the sweep asks whether a posting is still on its board, the gate asks whether we still
      crawl the employer, and a board goes uncrawled for far longer than a posting goes
      unlisted. Not an env var — see design.md.
- [ ] 2.3 Unit test the fold-and-credit mapping with a fake query runner (no database): two
      spellings of one employer both credited from one answer; an answer for a slug nobody
      asked about never appears in the result.
- [ ] 2.4 Integration test (`cmd/ingest`, alongside `board_health_integration_test.go`): the
      real query through the real adapter, asserting the freshness cutoff end-to-end.

## 3. Wiring

- [ ] 3.1 Replace `coverageLookup(cfg)` in `cmd/ingest/main.go` with `newCoverage(pool)` and
      rewrite its doc comment — the port is no longer Meili-backed and no longer keyed off
      `cfg.MeiliKey`.
- [ ] 3.2 **Note the behaviour change and check it:** the gate used to be silently OFF for any
      run without `MEILI_MASTER_KEY`, and is now on for every run that has a database. Confirm
      against `deploy/` which ingest units actually carry the Meili key today, so the change is
      reported as "gate now also applies to runs X, Y" rather than discovered later.
- [ ] 3.3 Update the `CoverageLookup` doc contract in `internal/ingest/pipeline/pipeline.go`:
      coverage means RECENT coverage, the implementation owns the window, and the paragraph
      about Meili being unable to compute the fold goes away — with it, the sentence conceding
      that hyphen-mismatched coverage is "missed by design" is no longer true.

## 4. Remove what this orphans

Verified as having no other reader: `chunkStrings` (0 uses outside the file), `foldSlug` (0),
`search.JobDocument.CompanySlugFolded` (2 uses, both in `document.go`). `InStrings`,
`NotInStrings`, `buildFacetResult` and `facetSearcher` are shared and stay.

- [ ] 4.1 Delete `internal/search/search/coverage.go` and its test, and
      `Client.NonAggregatorCompanies` with them.
- [ ] 4.2 Delete `JobDocument.CompanySlugFolded`, its assignment in `document.go`, and the
      `"company_slug_folded"` entry in the jobs index `FilterableAttributes` in `client.go`.
      Removing a filterable attribute needs no settings-before-binary dance — that hazard is
      one-directional (a binary querying an attribute the live index has not declared). The
      binary stops querying it first; the live index keeps a harmless declaration until the
      next rebuild drops it.
- [ ] 4.3 `go build ./... && go vet ./... && go test ./...`, then
      `go vet -tags=integration ./...`.

## 5. Documentation

- [ ] 5.1 `internal/ingest/pipeline/AGENTS.md`: rewrite the "exact slug OR folded, in one OR'd
      filter" bullet and the one after it about not crediting unasked slugs — both describe the
      Meili implementation. Replace with the freshness rule, the measurement, and the
      unrecoverable-vs-recoverable asymmetry that chose the window.
- [ ] 5.2 `internal/search/search/AGENTS.md` (and the `FilterableAttributes` comment): drop the
      `company_slug_folded` references now that the field is gone.
- [ ] 5.3 `docs/agents/job-lifecycle.md`: no change, but link the separate defect from the
      proposal — a board that leaves `sources/` takes its slugs out of the sweep's scope
      forever, which is what creates the stale rows this gate was trusting.

## 6. Verify on prod

- [ ] 6.1 File the follow-up issue for the never-closed rows of departed boards (11,151 slugs
      whose newest non-aggregator row is >30 days unseen). Out of scope here on purpose: this
      change stops those rows suppressing live postings; it does not close them.
- [ ] 6.2 After deploy, run `cmd/ingest` on `sources/himalayas.yml` and confirm
      `GET /api/v1/jobs/find?url=https://himalayas.app/companies/pipe/jobs/senior-software-engineer-5097528323`
      returns a `public_slug` instead of `{"data":null}` — the issue's own reproduction.
- [ ] 6.3 Read the run's `Stats.ATSCovered` against the previous run's. A fall is expected and
      is the measure of the fix; an unexpected collapse to near-zero would mean the freshness
      cutoff or the fold is wrong, not that the gate improved.
