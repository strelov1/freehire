## 1. Schema + backfill

- [ ] 1.1 Add migration `migrations/00NN_job_engagement_counts.sql`: `ALTER TABLE jobs ADD COLUMN view_count INT NOT NULL DEFAULT 0` and `applied_count INT NOT NULL DEFAULT 0`.
- [ ] 1.2 In the same migration, backfill both from `user_jobs`: `view_count` = count of the job's rows, `applied_count` = count filtered on `applied_at IS NOT NULL`.

## 2. Write path — increment counters (RED→GREEN per query)

- [ ] 2.1 Integration test (`internal/db`, `//go:build integration`): first `RecordJobView` sets `jobs.view_count` to 1; a repeat view by the same user leaves it at 1; a view by a second user makes it 2.
- [ ] 2.2 Rewrite `RecordJobView` in `internal/db/queries/user_jobs.sql` as the snapshot-CTE from design.md (prior → upsert → conditional `view_count` bump → `SELECT * FROM upsert`); keep the returned row shape identical.
- [ ] 2.3 Integration test: first `MarkJobApplied` sets `jobs.applied_count` to 1; marking applied again by the same user leaves it at 1; a second user makes it 2.
- [ ] 2.4 Rewrite `MarkJobApplied` as the snapshot-CTE (prior.applied_at → upsert → conditional `applied_count` bump on the NULL→set transition → `SELECT * FROM upsert`); preserve the stage-seeding and idempotency behavior.
- [ ] 2.5 `make sqlc`; commit regenerated `internal/db`. Confirm `go build ./...` and existing tracking tests still pass.

## 3. Wire shape

- [ ] 3.1 Test (`internal/jobview`): `FromRow` copies `view_count`/`applied_count` from the `db.Job` into the wire `Job`.
- [ ] 3.2 Add `ViewCount int32 \`json:"view_count"\`` and `AppliedCount int32 \`json:"applied_count"\`` to `jobview.Job` and populate them in `FromRow` (and `FromRows` inherits it).
- [ ] 3.3 Verify the fields appear on `GET /api/v1/jobs/:slug` (existing handler test or a quick integration assertion).

## 4. Frontend (detail page)

- [ ] 4.1 Add `view_count`/`applied_count` to the `Job` type in `web/src/lib/types.ts`.
- [ ] 4.2 In `web/src/lib/components/JobView.svelte`, render a muted "N views · M applied" line on the detail page; omit each metric when its count is 0, and render nothing when both are 0. Verify with `npx svelte-check`.

## 5. Verify

- [ ] 5.1 `go test ./...` (unit) green; `go test -tags=integration ./internal/db/` green (Docker).
- [ ] 5.2 `go build ./... && go vet ./...` and `npx svelte-check` clean.
- [ ] 5.3 Recreate the DB volume locally (`docker compose down -v && make up`) or apply the migration, then manually confirm: open a job while signed in → `view_count` increments once; apply → `applied_count` increments once; refresh → neither moves.
