## 1. Schema + backfill

- [x] 1.1 Add migration `migrations/0040_job_engagement_counts.sql`: `ALTER TABLE jobs ADD COLUMN view_count INT NOT NULL DEFAULT 0` and `applied_count INT NOT NULL DEFAULT 0`.
- [x] 1.2 In the same migration, backfill both from `user_jobs`: `view_count` = count of the job's rows, `applied_count` = count filtered on `applied_at IS NOT NULL`.

## 2. Write path — increment counters (RED→GREEN per query)

- [x] 2.1 Integration test (`internal/db`, `//go:build integration`): first `RecordJobView` sets `jobs.view_count` to 1; a repeat view by the same user leaves it at 1; a view by a second user makes it 2.
- [x] 2.2 Rewrite `RecordJobView` in `internal/db/queries/user_jobs.sql` as the snapshot-CTE from design.md (prior → upsert → conditional `view_count` bump → `SELECT * FROM upsert`); keep the returned row shape identical.
- [x] 2.3 Integration test: first `MarkJobApplied` sets `jobs.applied_count` to 1; marking applied again by the same user leaves it at 1; a second user makes it 2.
- [x] 2.4 Rewrite `MarkJobApplied` as the snapshot-CTE (prior.applied_at → upsert → conditional `applied_count` bump on the NULL→set transition → `SELECT * FROM upsert`); preserve the stage-seeding and idempotency behavior.
- [x] 2.5 `make sqlc`; commit regenerated `internal/db`. Confirm `go build ./...` and existing tracking tests still pass. (sqlc emits bespoke `RecordJobViewRow`/`MarkJobAppliedRow`; repository converts via `db.UserJob(row)`.)

## 3. Wire shape

- [x] 3.1 Test (`internal/jobview`): `FromRow` copies `view_count`/`applied_count` from the `db.Job` into the wire `Job`.
- [x] 3.2 Add `ViewCount int32 \`json:"view_count"\`` and `AppliedCount int32 \`json:"applied_count"\`` to `jobview.Job` and populate them in `FromRow` (and `FromRows` inherits it).
- [x] 3.3 Verify the fields appear on `GET /api/v1/jobs/:slug` (JSON contract test `TestJobJSON_ExposesEngagementCounts`).

## 4. Frontend (detail page)

- [x] 4.1 Add `view_count`/`applied_count` to the `Job` type — the type is generated (`make gen-contracts`) from `jobview.Job`, so regenerated `web/src/lib/generated/contracts.ts` rather than hand-editing `types.ts`.
- [x] 4.2 In `web/src/lib/components/JobView.svelte`, render a muted views/applied line in the sidebar metadata; omit each metric when its count is 0. Verified with `npx svelte-check` (0 errors).

## 5. Verify

- [x] 5.1 `go test ./...` (unit) green; `go test -tags=integration ./internal/db/` green (Docker, 245s, all pass).
- [x] 5.2 `go build ./... && go vet ./...` and `npx svelte-check` clean.
- [x] 5.3 Increment/idempotency behavior (first view→1, repeat→no change, second user→2; apply likewise) is proven against a real Postgres by `TestJobEngagementCounts` in the integration suite — stronger than a manual click. UI rendering verified by `svelte-check`. Live in-browser visual check deferred to deploy.
