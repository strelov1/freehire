## 1. Data & SQL layer

- [x] 1.1 Add migration `migrations/0022_emails_deleted_at.sql`: `ALTER TABLE emails ADD COLUMN deleted_at timestamptz` + partial index `emails_user_live_idx ON emails (user_id, received_at DESC) WHERE deleted_at IS NULL`
- [x] 1.2 Extend `ListEmails`/`CountEmails` in `internal/db/queries/gmail.sql`: exclude `deleted_at IS NOT NULL`, add optional `unread` (bool) and `status` (text) filters via the existing `sqlc.arg = <empty> OR predicate` idiom
- [x] 1.3 Add `MarkAllEmailsRead` (`:execrows`) taking the same source/unread/status/search args, updating only unread, non-deleted rows scoped to the user
- [x] 1.4 Add `SoftDeleteEmail` and `RestoreEmail` (`SET deleted_at = now()` / `= NULL`, scoped by `id + user_id`)
- [x] 1.5 Regenerate sqlc (`make sqlc`) and confirm `go build ./...`

## 2. Backend handlers & routes

- [x] 2.1 In `internal/handler/inbox.go`, extend `GetInbox` to read `?unread=1` and `?status=<signal>`, validating `status` against the mailclassify vocabulary (400 on unknown) and passing both into the query
- [x] 2.2 Add `MarkAllReadInbox` handler (reads the same query filters) + route `POST /me/inbox/read-all`
- [x] 2.3 Add `DeleteEmail` + `RestoreEmail` handlers + routes `POST /me/emails/:id/delete` and `POST /me/emails/:id/restore`, scoped to the caller (404 for another user's message)
- [x] 2.4 Integration test (build-tag, like `mailbox_integration_test.go`): `ListEmails` with `unread`/`status`/`deleted_at`; `MarkAllEmailsRead` honoring filters; soft-delete/restore round-trip

## 3. Frontend API client

- [x] 3.1 In `web/src/lib/api.ts`, add optional `unread`/`status` params to `getInbox`, and new methods `markAllRead`, `deleteEmail`, `restoreEmail`

## 4. Frontend toolbar filters

- [x] 4.1 In `InboxView.svelte`, add `unread`/`label` `$state` and wire an **Unread only** toggle + **All Labels** dropdown (from `emailStatus.ts` `STATUS_LABELS`) that call `reloadList()` on change; thread both into `fetchFirstPage()`
- [x] 4.2 Update the empty-state copy to "No mail matches your filters" when a filter is active

## 5. Frontend mark-all-read & delete/undo

- [x] 5.1 Add a **Mark all read** toolbar button: call `api.markAllRead(...)`, optimistically mark visible rows read, drop the unread count; on error `reloadList()`
- [x] 5.2 Add a **Delete** control (reading pane) with optimistic removal, `total` decrement, and a "Deleted · Undo" toast reusing the `lastUnlinked`/`undoUnlink` pattern (`lastDeleted` + `restoreEmail`)

## 6. Verify

- [x] 6.1 `go build ./... && go vet ./... && go test ./...`
- [x] 6.2 Web: `svelte-check` clean on changed files; toolbar verified on the deployed instance.
