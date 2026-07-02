## 1. Blob store abstraction (`internal/blobstore`)

- [x] 1.1 Add `minio-go` to `go.mod`; write tests for the `blobstore` config/nil-guard (unconfigured → nil) and key derivation (`resumes/<userID>`)
- [x] 1.2 Implement `Store` interface (`Put`/`Get`/`Delete`) + a `minio.Client`-backed impl built from `S3_ENDPOINT/S3_BUCKET/S3_ACCESS_KEY/S3_SECRET_KEY`; `New` returns nil when unconfigured
- [ ] 1.3 Integration-style test (build-tagged, MinIO testcontainer or skipped when no Docker) for a real Put/Get/Delete round-trip — DEFERRED to the live-bucket smoke (7.2)

## 2. Persistence (`users` résumé pointer)

- [x] 2.1 Migration `migrations/0039_users_resume_pointer.sql`: nullable `resume_object_key TEXT` + `resume_uploaded_at TIMESTAMPTZ`, with rollback comment
- [x] 2.2 sqlc queries: `SetUserResume` (key + now()), `ClearUserResume`, `GetUserResume` (pointer fetch); `make sqlc`
- [ ] 2.3 `accounts` (or a small `resume` service) use cases for set/clear/get-pointer, owner-scoped, with tests

## 3. Config + server wiring

- [ ] 3.1 Add `S3Endpoint/S3Bucket/S3AccessKey/S3SecretKey` to server config + `Load` (optional) with a test
- [ ] 3.2 In `cmd/server`, build the blobstore only when all four are set; pass into `handler.Config`; store on `API` (nil-safe)

## 4. Résumé endpoints + verdict rework

- [ ] 4.1 `PUT /api/v1/me/resume`: parse file, store to S3 under the user key, set the pointer, return metadata; degrade (400/501) cleanly when storage is unconfigured; tests
- [ ] 4.2 `GET /api/v1/me/resume` (metadata) + `DELETE /api/v1/me/resume` (remove object + clear pointer), owner-scoped; tests
- [ ] 4.3 Profile-form extraction path also stores the résumé to S3 when configured (single upload point), reusing the parsed text for skills
- [ ] 4.4 Rework `POST .../verdict`: read the stored résumé from S3 → parse → coherence (no request upload); when no résumé stored, return a state the UI shows as "upload once"; keep LLM/storage degradation; update tests

## 5. Frontend

- [ ] 5.1 `api.ts`: `putResume(File|string)`, `getResumeStatus()`, `deleteResume()`; verdict "re-run" calls the reworked endpoint
- [ ] 5.2 Verdict page: if a résumé is stored, show "Re-run coherence" (no dropzone nag); if not, one upload prompt; drop the "Add a coherence check / again" ambiguity
- [ ] 5.3 A "My résumé" indicator (stored + uploaded_at, replace/delete) — single upload point surfaced in the profile/verdict UI
- [ ] 5.4 `svelte-check` clean

## 6. Ops (freehire-ops)

- [ ] 6.1 Create the Hetzner Object Storage bucket `freehire-resumes` (hel1, private); document in `freehire-ops` README
- [ ] 6.2 Add `S3_ENDPOINT/S3_BUCKET/S3_ACCESS_KEY/S3_SECRET_KEY` to prod `/opt/freehire/.env` (and `.env.example`)

## 7. Verification

- [ ] 7.1 `go build/vet/test`; `make sqlc` committed; `go mod tidy` committed
- [ ] 7.2 Local end-to-end with a MinIO/hel1 test bucket: upload once → skills + stored; verdict re-runs coherence from storage; delete; degradation with S3 unset; `npm run check`
