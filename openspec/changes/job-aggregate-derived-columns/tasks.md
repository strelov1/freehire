## 1. Route the caller-supplied posted time through the draft

Must land before the mapping computes the content hash, or the two callers below
would fingerprint a NULL posted time while writing a real one.

- [x] 1.1 `cmd/tg-extract/store.go`: pass the Telegram post's timestamp through
  `job.Draft.PostedAt` (converting `pgtype.Timestamptz` → `*time.Time` at the draft
  boundary) instead of overwriting `params.PostedAt` after `UpsertParams()` returns.
- [x] 1.2 `internal/linkimport/linkimport.go`: pass `r.Job.PostedAt` through
  `job.Draft.PostedAt` instead of overwriting the mapped params.

## 2. The write mapping owns the derived columns

- [x] 2.1 `internal/job/job.go`: `Fields.UpsertParams` computes `ContentHash`
  (`jobhash.Of`) and `RoleFingerprint` (`jobhash.RoleFingerprint`) on the params it
  returns; rewrite the doc comment so it states the mapping owns every derived
  column instead of instructing callers to set them afterwards.
- [x] 2.2 Delete the now-redundant post-mapping assignments in `cmd/ingest/store.go`,
  `cmd/tg-extract/store.go` and `internal/linkimport/linkimport.go`.

## 3. The moderator create path

- [ ] 3.1 `internal/db/queries/jobs.sql`: add `content_hash` and `role_fingerprint` to
  `UpsertManualJob`'s insert column list and its `ON CONFLICT DO UPDATE` set; run
  `make sqlc`.
- [ ] 3.2 `internal/job/job.go`: `Fields.UpsertManualParams` fills both derived columns
  by fingerprinting `f.UpsertParams()`, so a manual write and an automated write of the
  same content produce the same two values.

## 4. The moderator edit path

- [ ] 4.1 `internal/db/queries/jobs.sql`: add `content_hash` and `role_fingerprint` to
  `UpdateManualJob`'s SET list — the edit is exactly when re-derived content moves the
  fingerprints; run `make sqlc`.
- [ ] 4.2 `internal/job/job.go`: add `Fields.UpdateManualParams(slug, actorID)` mirroring
  `UpsertManualParams`, and have `internal/moderation/repository.go`'s `Update` call it
  instead of hand-building the `db.UpdateManualJobParams` literal.

## 5. Verification

- [ ] 5.1 `go build ./... && go vet ./... && go test ./...` green; `openspec validate
  job-aggregate-derived-columns --strict` passes.
