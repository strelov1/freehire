## 1. The verdict classifier

- [x] 1.1 Add failing table-driven tests in `internal/ghost/classify_test.go` covering the level rules: one criterion → `none`; two structural → `possible`; two structural plus one contributor → `possible`; two contributors plus one other criterion → `likely`; two contributors alone → `possible`. Include a named test that pins the doctrine directly — structural criteria, however many fire, never produce `likely` — since no scenario test would catch that rule being inverted.
- [x] 1.2 Write `internal/ghost/classify.go`: an `Input` of plain scalars (reality class, absence stamp + validity, distinct contributors, silent applications, reports, `Now`), a `Result{Level, Criteria}`, and the level constants. No database, no Fiber, no clock of its own — mirror `internal/jobreality`'s shape and document the tiers at the point of definition, as the silence ladder documents its provenance.
- [x] 1.3 Add a failing test that an absence stamp older than 14 days does not fire `ats_absent`, then implement the expiry inside `Classify` so no caller can forget it.
- [x] 1.4 `go test ./internal/ghost/...` green.

## 2. Outcome evidence

The silence threshold ladder lives in `internal/userjob/silence.go` as five numbers carrying their
provenance. Restating it in SQL would let a change to it disagree silently with the personal
tracking board — the same application judged by two ladders on two surfaces, with nothing binding
them. So the split is: **SQL selects candidates and holds the mailbox gate** (a join belongs
there), **Go applies the ladder and counts distinct people** (a rule belongs where its single
source is). The aggregator is then testable without a database.

- [x] 2.1 Add failing tests in `internal/ghost/evidence_test.go`: an application inside its stage's threshold contributes nothing; one past it contributes; a terminal stage contributes nothing; an application with a pending suggestion contributes nothing, since unconfirmed mail makes the silence a question rather than a fact.
- [x] 2.2 Add a failing test that one user contributing both a silent application and a report counts as **one** contributor, and that the returned map holds only jobs with at least one unit — sparse by construction rather than by the caller filtering.
- [x] 2.3 Add a failing test that a report matures only once the `applied` threshold has passed since its stated apply date, reading that number from `userjob.SilenceThresholdDays` rather than restating it.
- [x] 2.4 Write `internal/ghost/evidence.go`: the aggregator over candidate rows, returning per-job contributors/silent-applications/reports for `Classify`.
- [x] 2.5 Add a failing integration test in `internal/db`: the candidate query returns an application only when its owner has a connected mailbox — a `connected` Gmail row or an allocated hosted mailbox — and omits it otherwise, whatever its silence.
- [x] 2.6 Add `ListGhostApplicationEvidence` and `ListGhostReportEvidence` to `internal/db/queries/`, mirroring the tracking query's `last_activity_at` and `has_pending_suggestion` derivation rather than inventing a second definition of them. Run `make sqlc`.
- [x] 2.7 `go test ./internal/ghost/...` and `go test -tags=integration ./internal/db/` green.

## 3. The report channel

- [x] 3.1 Migration `0051_ghost_job_signal.sql` — DONE IN GROUP 2, which needed the table for `ListGhostReportEvidence`. Creates `ghost_reports` (`UNIQUE (user_id, job_id)`, `applied_on`, `retracted_at`, cascading FKs, partial index on live rows) and `jobs.ats_absent_at`. One migration: they ship together, and the column must exist before any generated `SELECT` reads it.
- [x] 3.2 Add failing tests in `internal/ghostreport`: `applied_on` in the future or over 12 months old is invalid; a claim under 21 days old is stored but yields no evidence; a retracted report yields none.
- [x] 3.3 Write `internal/ghostreport` — a Fiber-free, pgx-free service owning validation, the maturity rule and retraction, over a repository interface. Follow `internal/report`'s shape.
- [x] 3.4 Add failing handler integration tests in `internal/handler`: 201 on file; 409 on a duplicate; 409 on a closed job; 403 for an unverified email; 429 past the daily cap; 204 on retract; unauthenticated is 401.
- [x] 3.5 Write `internal/handler/ghost_reports.go` and register `POST`/`DELETE /jobs/:slug/ghost-report` behind `RequireAuthOrKey`. Retraction returns **204**, never `SendStatus(200)` — that writes the body `OK` and breaks JSON clients on a call that succeeded.
- [x] 3.6 Add the queries to `internal/db/queries/ghost_reports.sql` and run `make sqlc`; map the unique violation to the conflict error in the repository, as `job_reports` does.
- [x] 3.7 `go test ./internal/ghostreport/... ./internal/handler/...` and the integration suite green.

## 3b. The report dialog

Added after the dialog was reviewed against this work: `no_response` is the ghost intent, and it
has been landing in a queue whose only verdict is closing the job.

- [x] 3b.1 Add a failing test in `internal/report` that a report with blank or absent `details` is accepted, and that over-long details are still refused. Then drop the required-details rule from `FileInput.validate`.
- [x] 3b.2 Update `openspec` — done: `job-report` gains the optional-`details` requirement, `ghost-job-signal` gains the dialog-routing requirement.
- [x] 3b.3 In `ReportDialog.svelte`, route `no_response` to a date step that files a ghost report; leave the other four reasons on the moderation path.
- [x] 3b.4 Make "What's wrong?" optional in the dialog ("Tell us more"), drop its `*`, and remove the "Telegram for follow-up" field. The API keeps `contact_telegram` — it is documented and reachable by third-party clients, so it is not orphaned by the SPA dropping it.
- [x] 3b.5 `go test ./internal/report/... ./internal/handler/...` and `pnpm test && pnpm lint` in `web/` green.

## 4. The ATS cross-check

- [x] 4.1 Add a failing test for the role key: an aggregator posting whose description was truncated still matches the company board's full-text posting under the same title; a per-city title variant matches its base role. Reuse `jobhash.stripTrailingClause`/`normalizeRoleText` rather than restating the normalization — export what is needed.
- [x] 4.2 Add a failing integration test for the coverage gate: a company present only on aggregator sources is never stamped, however unmatched its titles.
- [x] 4.3 Add the queries: candidate aggregator jobs by keyset, and a company's open role keys from sources of kind `ats`/`company` (`sources.ProviderKind` decides the kinds; the query takes the resolved provider list). Run `make sqlc`.
- [x] 4.4 Write `cmd/ghost-crosscheck`: run-once-and-exit, `DATABASE_URL`, keyset over open aggregator jobs grouped by company, one role-key query per company, stamping and clearing `ats_absent_at`. **Dry-run by default**, `--apply` to write — the `cmd/prune` discipline. The dry run prints the calibration report: how many jobs would reach each level, broken down by source and by company.
- [x] 4.5 Add a failing test that a run stamps and a subsequent run clears when the role reappears on the company's board, so the stamp tracks the world rather than accumulating.
- [x] 4.6 `go test ./cmd/ghost-crosscheck/...` green and `go test -tags=integration ./internal/db/` still green.

## 5. Serving the signal

- [x] 5.1 Add failing tests in `internal/jobview`: a closed job carries no ghost field; a job below the anonymity gate carries the level and criteria but **no** count fields; a job above it carries the distinct-contributor count.
- [x] 5.2 Write `jobview.Ghost` and `ClassifyGhost(job, now, evidence)` following `ClassifyReality`'s row-based shape and its documented reasoning, with `omitempty` on the field and on the gated counts.
- [x] 5.3 Wire the bulk evidence lookup into the job detail, job list AND SEARCH read paths, hydrating sparse maps per request rather than a query per card. Search was added beyond the original task: it is the surface where cards are actually browsed, so omitting it would have left the badge almost nowhere. It needs a Postgres read for `ats_absent_at` because reindex is `content_hash`-incremental and a column no adapter writes never reaches the index — the `is_tech` trap. Also added `RoleClusterCountsFor`, a page-scoped bulk variant, so the list does not issue a cluster query per card.
- [x] 5.4 Regenerate the web contracts (`make` target per `web/AGENTS.md`) so `Ghost` reaches `web/src/lib/generated/contracts`.
- [x] 5.5 `go test ./internal/jobview/... ./internal/handler/...` green.

## 6. The interface

- [x] 6.1 Add failing unit tests in `web/src/lib/ghost.test.ts` for the presentation helper: hedged wording per level, the `N/M` scale, and the checklist rows including the "no data" state for criteria that did not fire.
- [x] 6.2 Write `web/src/lib/ghost.ts` and `GhostBadge.svelte` (chip + scale) alongside the existing `RealityBadge`, matching its facts-not-accusation structure.
- [x] 6.3 Write `GhostChecklist.svelte` for the job page: every criterion, fired ones with their facts, unfired ones explicitly without data.
- [x] 6.4 Add a failing test that the ghost chip supersedes the reality chip when both would render, and that reality renders unchanged when ghost is `none`. Then wire both into `JobView.svelte` and the card.
- [x] 6.5 Add the report control to the job page — "I applied, no answer" plus a date field — visible only to a signed-in user, wired to the endpoint with its conflict and rate-limit states surfaced.
- [ ] 6.6 `pnpm test` and `pnpm lint` green in `web/`; eslint is a required CI gate, and oxlint passing does not imply it.

## 7. Company response rate

- [x] 7.1 Add a failing integration test: a company with applications below the sample gate serves no rate; above it serves the rate; applications from users without a connected mailbox count on neither side.
- [x] 7.2 Added `insights_company_response` — a SCALAR table beside `insights_company_growth`, not columns on `insights_company_stats`, which is a per-day series and the wrong shape. Rebuilt by `cmd/rollup-company` in the same transaction. Growth and response answer unrelated questions (how fast a company hires vs. how it treats applicants) and a company can look excellent on one while looking terrible on the other, so folding them together would invite reading one as evidence for the other.
- [x] 7.3 `go test -tags=integration ./internal/db/` and `go test ./...` green.

## 8. Whole-change verification

- [ ] 8.1 `go build ./... && go vet ./... && gofmt -l .` clean.
- [ ] 8.2 `go test ./...` green; `go test -tags=integration ./internal/db/` green.
- [ ] 8.3 Confirm no Meilisearch surface changed — no new filterable attribute, no index settings diff. The v1 verdict is read-time by design and a facet would open a hard-500 window until a rebuild swaps in.
- [ ] 8.4 Verify the structural feature flag holds: on a database with the migration applied and no crosscheck run and no reports, no job in the catalogue reaches a level above `none`.

## 9. Rollout (ops — executed at Finish)

- [ ] 9.1 Apply migration `0051` to prod **before** deploying the image. The generated `SELECT`s read `jobs.*`, so an unapplied migration 500s every job read, not only this feature.
- [ ] 9.2 Deploy. Confirm the catalogue is unchanged — the feature is silent until 9.4.
- [ ] 9.3 Run `cmd/ghost-crosscheck` by hand, dry-run, and capture the calibration report.
- [ ] 9.4 **Calibration gate.** Read the report: who reaches `possible`, by source and by company. If staffing/consulting agencies and close-event-less enterprises dominate, STOP — the previous spike died exactly here, and the next step is a dict-only exclusion specified as its own change, not a lowered threshold. Only on passing does the cron start and can a mark first appear.
- [ ] 9.5 A week after the cron starts, re-run the report and record what moved, so the numbers in the design have a measured successor rather than a founding estimate.
