## Why

The `/my/inbox` surface is a read-only aggregator with only account-switch and
search controls. As a user's ATS mail piles up, they need to triage it: hide
what they've read, focus one classification, clear the unread pile, and remove
noise. These are all read-side operations we can build on data we already have
(`read_at`, `status_signal`) plus one soft-delete flag.

## What Changes

- Add an **Unread only** filter to the inbox listing (messages with no `read_at`).
- Add an **All Labels** filter that narrows the listing to one `status_signal`
  (the existing mailclassify classification), validated against the vocabulary.
- Add **Mark all read**: a bulk action that marks every message matching the
  currently active filters (source / unread / label / search) as read.
- Add **Delete** as a soft-delete (`deleted_at`): deleted mail is hidden from the
  listing and offers an inline **Undo** (restore), mirroring the existing
  unlink-Undo pattern. No separate "Deleted" folder.

Out of scope (explicitly deferred): Priority sort, outbound mail
(Reply/Forward/Sent), and a folder dropdown.

## Capabilities

### New Capabilities

<!-- none -->

### Modified Capabilities

- `email-inbox`: the inbox listing gains unread and label filters; a bulk
  mark-all-read action; and a per-message soft-delete with Undo.

## Impact

- Migration: `emails` gains a nullable `deleted_at` column + a partial index on
  live rows.
- SQL (`internal/db/queries/gmail.sql`): `ListEmails`/`CountEmails` exclude
  soft-deleted rows and accept `unread`/`status` filters; new `MarkAllEmailsRead`,
  `SoftDeleteEmail`, `RestoreEmail` queries.
- API (`internal/handler/inbox.go` + routes): `GetInbox` accepts `unread` and
  `status`; new `POST /me/inbox/read-all`, `POST /me/emails/:id/delete`,
  `POST /me/emails/:id/restore`.
- Web (`web/src/lib/api.ts`, `web/src/lib/components/InboxView.svelte`): new
  toolbar controls and Delete/Undo wiring; reuses `emailStatus.ts` vocabulary.
