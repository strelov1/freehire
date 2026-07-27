## 1. Schema: de-authored community content

- [x] 1.1 Add `migrations/0041_threads_author_set_null.sql`: drop `NOT NULL` on `threads.author_user_id` and recreate `threads_author_user_id_fkey` as `ON DELETE SET NULL`
- [x] 1.2 Change every `community_personas` join in `internal/db/queries/community.sql` from `JOIN` to `LEFT JOIN` (thread read, both list queries, reply reads) so a de-authored thread still appears; regenerate with `make sqlc`
- [x] 1.3 Render a de-authored author as a marker distinct from `aiAuthor` in `internal/handler/community.go` (thread and reply responses), covered by a handler test asserting a deleted author is not labelled "AI"

## 2. Session termination

- [x] 2.1 ~~Add a `UserExists` query~~ — dropped: session revocation landed on `main` first, and its `token_version` load answers the same question in the same round-trip
- [x] 2.2 Cover the guarantee where it lives: tests in `internal/auth` assert `401` for a token whose account is gone, on both `RequireAuth` and the cookie path of `RequireAuthOrKey` (the key path needs none — `api_keys` cascade)
- [x] 2.3 ~~Wire the checker at every construction site~~ — not needed: `resolveSession` already fails closed on an unreadable token version
- [x] 2.4 Update `internal/auth/AGENTS.md`: the fail-closed version load is what terminates a deleted account's sessions, and must not be softened

## 3. Erasure orchestration (`internal/accountdelete`)

- [x] 3.1 Add the SQL the service needs to `internal/db/queries/users.sql`: `DeleteUser`, `ListUserEmailObjectKeys` (non-null `emails.s3_key`), `ListUserReferralProofKeys` (`referral_offers.proof_object_key`); regenerate
- [x] 3.2 Create `internal/accountdelete` with `Service.Delete(ctx, userID)` and its narrow dependency interfaces (repository, blob store, Gmail revoker); test-drive the ordering: keys collected → revoke → objects deleted → rows deleted
- [x] 3.3 Test: a blob-store failure aborts before any row is deleted and surfaces a retryable error
- [x] 3.4 Test: a revoke failure is logged and does not stop the deletion
- [x] 3.5 Test: nil blob store (storage unconfigured) and nil revoker (Gmail unconfigured) both delete successfully

## 4. Endpoint

- [x] 4.1 Add `internal/handler/me_delete.go`: `DELETE /api/v1/me`, cookie-only, case-insensitive confirmation of the caller's own email, `204` plus an expired session cookie on success, `400` on mismatch
- [x] 4.2 Register the route under `auth.RequireAuth` (never `keyAuth`) in `internal/handler/handler.go`; test that a Bearer API key gets `401`
- [x] 4.3 Integration test (`-tags=integration`): seed a user with job tracking, a CV, credits, saved searches, mail, a thread with another member's reply, and a referral offer; delete; assert no user-owned row survives, the other member's reply survives de-authored, the moderator trail is nulled, and the objects are gone from the fake store
- [x] 4.4 Integration test: the deleted account's email can register a fresh, empty account
- [x] 4.5 Document the endpoint in `internal/handler/AGENTS.md`

## 5. Web surface

- [x] 5.1 Add the delete-account danger zone to `web/src/routes/my/profile`: states that deletion is permanent and unrecoverable, lists what is erased (CV, mail, analyses, credits, community handle), and notes what survives de-authored
- [x] 5.2 Gate the action on the typed email matching the signed-in address; keep it disabled until it does
- [x] 5.3 On success clear client session state and redirect to the public site; verify visually per `web/AGENTS.md`

## 6. Ship

- [x] 6.1 `go build ./... && go vet ./... && go test ./...`, plus the integration tag suite for the new tests
- [x] 6.2 Note in the change that migration 0041 must be applied by hand on prod BEFORE the binary deploys
- [x] 6.3 Offer a changelog entry (`write-changelog`) — this is user-facing
