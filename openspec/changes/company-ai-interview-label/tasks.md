## 1. Storage

- [x] 1.1 Add the migration: `company_process_reports` (`user_id` NOT NULL cascading, `company_slug` FK cascading, `kind` with a `CHECK` list holding `ai_interview`, `created_at`, `retracted_at`, `UNIQUE (user_id, company_slug, kind)`) and `companies.ai_interview_reports integer NOT NULL DEFAULT 0`. Carry the "APPLY TO PROD MANUALLY BEFORE DEPLOY" header 0088 uses — initdb runs migrations only on first volume init.
- [x] 1.2 Run `pnpm check:sql` on the new file and fix what squawk reports on it (the applied history's findings stay).
- [x] 1.3 Write the queries in `internal/platform/db/queries/`: file (upsert clearing `retracted_at`), retract, count un-retracted per company+kind, and recompute the materialised counter. Run `make sqlc`.
- [x] 1.4 Integration test (`//go:build integration`) that the uniqueness constraint rejects a second row and that re-filing after retraction reuses the row rather than inserting.

## 2. Domain service

- [x] 2.1 Create `internal/engage/processreport` with the controlled `kind` vocabulary as a code constant, and add the package to the table in `internal/platform/arch/layering/blocks.go` — a package in neither block table fails the guard.
- [x] 2.2 File: reject an unknown kind before any write; 409 on an existing un-retracted row for the same `(user, company, kind)`; clear `retracted_at` when one exists; recompute the counter in the same transaction.
- [x] 2.3 Retract: set `retracted_at` without deleting, recompute the counter in the same transaction, and answer "not found" when the user never filed.
- [x] 2.4 Unit tests for the vocabulary gate, the duplicate answer, the re-file-after-retraction path, and that the counter equals the un-retracted row count after each operation.

## 3. HTTP

- [x] 3.1 `POST /api/v1/companies/:slug/process-reports` — `RequireAuth`, 404 on an unknown company slug, 400 on an unknown kind, 409 on a duplicate, 201 on success. Response shape `{"data": ...}`.
- [x] 3.2 `DELETE /api/v1/companies/:slug/process-reports` — retracts the caller's own report of the named kind; 404 when none exists.
- [x] 3.3 Apply the same per-day rate limit the report endpoints answer 429 on.
- [x] 3.4 Integration tests in `internal/api/handler` covering every status above, including that filing creates no moderation report.

## 4. Wire shape

- [x] 4.1 Add the label + count to `jobview.Job`, omitted entirely when the count is zero (never a zero on the wire), and to the card input struct alongside `Collections`. Keep it scalars-in: `job` (layer 5) may not import `engage` (layer 7), so the value arrives from the query, never from the service.
- [x] 4.2 Add `jobs.ai_interview_reports` (migration 0156) and sync it from the company inside the report transaction — one `UPDATE ... WHERE company_slug = $1` beside the recompute, bumping `updated_at` so `reindex --since` carries it. See design.md, "The counter is a `jobs` column, synced inside the report transaction".
- [x] 4.3 Add the same field to the company read used by the company page.
- [x] 4.4 Unit tests: a labelled company's job carries label + count; an unlabelled one omits the field.

## 5. Search

- [x] 5.1 Add the field to `JobDocument` and declare it filterable in the jobs index settings (`internal/search/search/client.go`).
- [x] 5.2 Add the query param to `search.UnknownParams`' vocabulary so it is never silently dropped, and map it in `query_filter.go`.
- [x] 5.3 Confirm `search-settings-drift` reports the gap when the attribute is missing from the live index — this is the guard that makes the ordering mistake visible.
- [x] 5.4 Integration test (`//go:build integration`) that the filter excludes a labelled company's jobs and that omitting it returns both.

## 6. Frontend — filing

- [x] 6.1 Add the entry to `reportReasons` in `web/src/lib/reports.ts` and extend the evidence split so it routes to the company endpoint, not the moderation one.
- [x] 6.2 Extend the test that pins the split so routing this entry to the moderation queue fails a test by name.
- [x] 6.3 Wire `ReportDialog.svelte`: choosing it submits immediately from the company slug the job payload already carries — no date step, no details step.
- [x] 6.4 Add the API client methods and their error mapping (409 "you already reported this", 401, 429).

## 7. Frontend — showing

- [x] 7.1 A badge component rendering the practice and its count in neutral styling — no warning colour, no alert icon. It MUST NOT render when the count is absent.
- [x] 7.2 Place it on the job card, the job page and the company page.
- [x] 7.3 Add the filter control to the filter modal so it is reachable with a mouse, and make it persist the way the other filters do.
- [x] 7.4 Component tests: badge hidden at zero, badge shows the count, filter round-trips through the URL.

## 8. Ship

- [x] 8.1 `gofmt -w` the touched Go, then `go vet ./...`, `go test ./...`, and `go vet -tags=integration ./...` before pushing.
- [x] 8.2 ~~Apply the migration on prod by hand~~ — NOT needed: `deploy/bin/release.sh` builds and runs `cmd/migrate` itself, idempotently and under an advisory lock, BEFORE the new colour starts, and a migration failure aborts the release with the live colour untouched. The migration file's "apply manually" header describes the Docker `initdb` path, which is a different one.
- [x] 8.3 Patched the live Meilisearch jobs index: 33 filterable attributes, `ai_interview` among them (task 755175, 166s). Applied only AFTER `freehire-reindexw` finished — a rebuild swaps a fresh index carrying the deployed binary's settings over the live one, which would have discarded the patch.
- [x] 8.4 Deployed 7de312ca8 and verified the empty state on prod: `POST /api/v1/companies/:slug/process-reports` answers 401 (route live), `/jobs/facets` and its disjunctive path 200, `ai_interview=false` returns the whole catalogue (2,045,297) and `ai_interview=true` returns 0, with `meta.ignored_params` absent — the param is read, not silently dropped. The company payload omits the count at zero, as required.
- [ ] 8.5 **Deferred by design, not forgotten.** A full `make reindex` is only needed once the first labels land: incremental pushes do not carry this field (it is not in `content_hash`), so pre-existing documents keep `ai_interview` absent until a rebuild. Nobody has reported anything yet, so there is nothing for a rebuild to carry. Stop `freehire-reindexw.timer` first.


## 9. What the rollout actually cost

Recorded because the plan above was wrong twice, and both corrections are reusable.

- **`ALTER TABLE jobs ADD COLUMN` could not catch a lock gap for ~3 hours.** It takes
  ACCESS EXCLUSIVE, which conflicts with the ACCESS SHARE of an ordinary `SELECT`, and
  `jobs` is never quiet. Every attempt failed `55P03` against the runner's deliberate 5s
  `lock_timeout` — and while it failed, **autodeploy was blocked for everybody**, not just
  this change.
- **Pausing the worker fleet was necessary but not sufficient.** `freehire:pause:all` holds
  only NEW runs; two `similar-backfill` queries already in flight held the table for 133s
  and counting. The migration went through within seconds of polling `pg_locks` until no
  holder older than 8s remained. Diagnosing with `pg_locks` should have come before the
  retries, not after six of them.
- **`cmd/migrate` respects the pause too**, which is what `FREEHIRE_IGNORE_PAUSE=1` is for:
  hold the fleet, then run one thing against a quiet host. It is documented in
  `worker/pause.go` and it is exactly this situation.
- **Autodeploy stops retrying a commit after two failures** (`/var/lib/freehire/autodeploy.state`,
  "not retrying until main moves"). Clearing that file is the way back, not pushing a commit
  to move `main`.
