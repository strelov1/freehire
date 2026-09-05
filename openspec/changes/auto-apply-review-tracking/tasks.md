## 1. SQL layer

- [ ] 1.1 Add `ListAutoApplyQueueForUser` to `internal/platform/db/queries/auto_apply_queue.sql`: all rows for `user_id`, joined to `jobs` for the minimal card fields (slug, title, company name/slug), ordered `created_at DESC`.
- [ ] 1.2 Add `ListRecentAutoAppliedForUser` to `internal/platform/db/queries/application_events.sql`: rows where `user_id = $1 AND event_type = 'applied' AND source = 'system'`, joined to `jobs`, `ORDER BY occurred_at DESC LIMIT 20`. Comment the query referencing this change's design.md for why `source = 'system'` is the auto-apply marker.
- [ ] 1.3 Run `make sqlc` and confirm `internal/platform/db` regenerates cleanly.

## 2. Backend use case

- [ ] 2.1 In `internal/application/autoapply`, add the six-value status derivation (`tailoring`/`pending_review`/`approved`/`blocked`/`declined`/`failed`) as a pure function over `(tailoredCVID, reviewDecision, blockedAt, failedAt)`, mirroring `autoApplyEntryStatus`'s existing precedence (declined checked before blocked/failed).
- [ ] 2.2 Add a use case function that calls both new queries and assembles the list result: pending attempts (status + job card + `unmapped` when `blocked`, never `last_error`) and recently-applied entries (job card + `applied_at`).
- [ ] 2.3 Unit-test the status derivation function directly (table-driven, one case per status plus the declined-vs-blocked precedence case from the spec).

## 3. Backend handler + route

- [ ] 3.1 Add `internal/api/handler/auto_apply_list.go` with `GetAutoApplyList`, calling the new use case, returning `{"data": {"pending": [...], "recently_applied": [...]}}`.
- [ ] 3.2 Mount `GET /me/auto-apply` behind `mw.cookie` in `internal/api/handler/assistant.go` (or wherever the sibling `/me/auto-apply/:queueId/...` routes are registered), next to the existing auto-apply routes.
- [ ] 3.3 Add an integration test (`//go:build integration`) covering: empty state, each of the six statuses, `unmapped` present only when blocked, `last_error` never serialized, recently-applied excludes a manually-marked application, an unauthenticated request is rejected.

## 4. Frontend API client

- [ ] 4.1 Add types for the new response shape to `web/src/lib/types.ts` (or generated contracts, per this repo's existing convention for wire shapes).
- [ ] 4.2 Add a `listAutoApply()` method to `web/src/lib/api.ts` calling `GET /api/v1/me/auto-apply`.
- [ ] 4.3 Confirm `POST /me/auto-apply/:queueId/review` already has (or add) an `api.ts` wrapper the new page can call for approve/decline.

## 5. Frontend page

- [ ] 5.1 Create `web/src/routes/my/auto-apply/+page.svelte`: sections for "needs attention" (`tailoring`/`pending_review`/`approved`/`blocked`), "declined / failed" (terminal, no actions), and "recently applied".
- [ ] 5.2 `pending_review` cards render inline Approve/Decline buttons calling the review endpoint, with optimistic removal from the pending list on success.
- [ ] 5.3 `blocked` cards render the `unmapped` question list; copy makes clear the attempt is final for this job (no retry implied), per design.md's risk note.
- [ ] 5.4 Empty state: no auto-apply activity yet, with a link back to job search.
- [ ] 5.5 Add the `/my/auto-apply` entry to `accountLinks` in `web/src/lib/components/HeaderMenu.svelte`, placed after `/my/cvs`.

## 6. Frontend tests + verification

- [ ] 6.1 Unit-test any status-to-badge/copy mapping helper, following `autoApplyButton.test.ts`'s pattern.
- [ ] 6.2 Manually drive the page in the browser: approve, decline, and a blocked entry's question list render correctly; verify against the golden path and the empty state.
- [ ] 6.3 `gofmt -w`, `go vet ./...`, `go test ./...`, and the frontend's `pnpm check` all pass before commit.
