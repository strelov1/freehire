## 1. `internal/job/dictgap` package

- [x] 1.1 Add `dictgap.SkillGapCandidate` and `SkillGapCandidates(counts map[string]int) []SkillGapCandidate`: normalizes each key (case/punctuation/whitespace), sums colliding buckets keeping the most-frequent original spelling as the display form, drops any phrase `skilltag.Parse` resolves to a non-empty result, sorts descending by count.
- [x] 1.2 Add `dictgap.TitleClassification` (`Title string; Count int; EnrichmentSeniority, EnrichmentCategory string`), `dictgap.DriftCandidate`, and `dictgap.DriftReport` (`Seniority, Category []DriftCandidate`).
- [x] 1.3 Add `dictgap.ClassifyDriftCandidates(rows []TitleClassification) DriftReport`: recomputes `classify.Parse(row.Title)`, emits a seniority candidate when it differs from `EnrichmentSeniority` (both non-empty) and a category candidate when it differs from `EnrichmentCategory` (both non-empty), each sorted descending by count.
- [x] 1.4 Register `internal/job/dictgap` in `internal/platform/arch/layering/blocks.go` (job block, layer 5).
- [x] 1.5 Unit tests for both functions: unresolved-phrase inclusion, resolved-phrase exclusion, case/punctuation/whitespace collapsing with count summing, seniority-only drift, category-only drift, agreement producing no candidate, empty enrichment value producing no candidate.

## 2. sqlc queries

- [x] 2.1 Add `SkillGapReportBounds :one` (`MIN/MAX(id)` over `jobs`, unfiltered — same shape as `RequirementsDerivedBackfillBounds`) to `internal/platform/db/queries/jobs.sql`.
- [x] 2.2 Add `ListJobSkillsForGapReport :many` (id range + row `LIMIT`, `enriched_at IS NOT NULL`, selecting `id` and the `enrichment->'skills'` array) to the same file.
- [x] 2.3 Add `ClassifyDriftReportBounds :one` (same `MIN/MAX(id)` shape) to the same file.
- [x] 2.4 Add `ListTitlesForClassifyDrift :many` (id range, no row `LIMIT` — an aggregated row has no id to resume from, so the range width alone bounds the statement —, `enriched_at IS NOT NULL`, `GROUP BY title` returning `title`, `count(*)`, and one representative `enrichment->>'seniority'`/`enrichment->>'category'` pair) to the same file.
- [x] 2.5 Run `make sqlc` and commit the regenerated `internal/platform/db` output.

## 3. `cmd/report-skill-gaps`

- [x] 3.1 `worker.Bootstrap`, read `REPORT_SKILL_GAPS_CHUNK` (default matching `backfill-requirements`'s id-range sizing rationale), `REPORT_SKILL_GAPS_TOP` (default 200), `REPORT_SKILL_GAPS_FROM_ID` via `worker.EnvInt64`.
- [x] 3.2 Chunked id-range loop over `SkillGapReportBounds`/`ListJobSkillsForGapReport` (resume-on-full-chunk, paced sleep between chunks, periodic progress log) — mirror `cmd/backfill-requirements`'s loop shape, folding every returned skill phrase into a `map[string]int`.
- [x] 3.3 After the scan, call `dictgap.SkillGapCandidates`, print the top-N as a tab-separated `count\tphrase` table to stdout.
- [x] 3.4 Doc comment at the top of `main.go` stating it is read-only, hand-run, `DATABASE_URL`-only — no `--apply`, no timer.
- [x] 3.5 `main_test.go` covering the chunk-loop resume/pacing logic against a fake/stub query source, following `cmd/backfill-requirements/main_test.go`'s pattern.

## 4. `cmd/report-classify-drift`

- [x] 4.1 `worker.Bootstrap`, read `REPORT_CLASSIFY_DRIFT_CHUNK`, `REPORT_CLASSIFY_DRIFT_TOP` (default 200), `REPORT_CLASSIFY_DRIFT_FROM_ID` via `worker.EnvInt64`.
- [x] 4.2 Chunked id-range loop over `ClassifyDriftReportBounds`/`ListTitlesForClassifyDrift`, accumulating `dictgap.TitleClassification` rows (a title seen in more than one chunk — id ranges don't align with `GROUP BY title` boundaries — must have its counts merged, not overwritten).
- [x] 4.3 After the scan, call `dictgap.ClassifyDriftCandidates`, print the top-N seniority candidates then the top-N category candidates as tab-separated tables (`count\ttitle\tdictionary_value\tenrichment_value`) to stdout.
- [x] 4.4 Doc comment at the top of `main.go`, same read-only/hand-run/`DATABASE_URL`-only statement as 3.4.
- [x] 4.5 `main_test.go` covering the chunk-loop and the cross-chunk title-merging logic.

## 5. Documentation

- [x] 5.1 Add both commands to the Worker gotchas list in `AGENTS.md`, following the existing bullet style (one-off, hand-run, read-only, `DATABASE_URL` only, no reindex needed).
- [x] 5.2 Add a short `internal/job/dictgap/AGENTS.md` if the package's rationale (recompute vs. stored column, why phrases aren't semantically clustered) isn't otherwise obvious from code comments — otherwise fold that rationale into doc comments and skip a separate file.

## 6. Verification

- [x] 6.1 `gofmt -l .` clean, `go vet ./...`, `go test ./...`.
- [x] 6.2 `go vet -tags=integration ./...`.
- [x] 6.3 Manually run both commands against a local/dev database and confirm the printed report is sane and no row is written (`git diff` on the DB is not a thing — confirm via `SELECT` before/after, or by code inspection that the commands issue no `INSERT`/`UPDATE`/`DELETE`).
