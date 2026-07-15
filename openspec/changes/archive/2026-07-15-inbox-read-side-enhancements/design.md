## Context

`/my/inbox` is a read-only aggregator over the shared `emails` table (Gmail sync
+ hosted SES intake). `ListEmails`/`CountEmails` already filter by `user_id`, an
optional `src`, and an optional `q`, ordered `received_at DESC`. `read` is
derived (`read_at IS NOT NULL`), not a stored boolean; `status_signal` holds the
mailclassify label. The frontend `InboxView.svelte` toolbar is `[account
switcher] [search] [Refresh]` and already carries an optimistic Undo pattern for
unlink. Full brainstormed spec:
`docs/superpowers/specs/2026-07-15-inbox-read-side-enhancements-design.md`.

## Goals / Non-Goals

**Goals:**
- Triage controls over existing data: unread filter, label filter, bulk
  mark-all-read, and per-message soft-delete with Undo.
- Reuse existing patterns (optional `sqlc.arg = '' OR ...` filters, the unlink
  Undo UX, the `emailStatus.ts` vocabulary). Minimal new surface.

**Non-Goals:**
- Priority sort, outbound mail (Reply/Forward/Sent), a folder dropdown, and a
  persistent "Deleted" folder view.

## Decisions

**Soft-delete via a `deleted_at` column, not a status enum or hard delete.**
A nullable timestamp is the least invasive: listing excludes `deleted_at IS NOT
NULL`, restore clears it. Hard delete was rejected because a deleted Gmail
message would reappear on the next sync; a status enum was overkill for a binary
state. Gmail's `UpsertEmail` is `ON CONFLICT DO NOTHING`, so re-sync never clears
the flag — soft-delete is durable with no extra guard.

**Filters ride the existing optional-arg idiom.** `unread` and `status` extend
`ListEmails`/`CountEmails` with `(sqlc.arg(x) = <empty> OR predicate)`, matching
how `src`/`q` already work. No new query shape, and the count stays consistent
with the listing.

**Mark-all-read respects active filters, not the whole mailbox.** The bulk
`UPDATE` takes the same source/unread/label/search args as the listing, so "mark
all read" means "everything currently shown". This is more predictable than a
blind whole-mailbox mark and avoids surprising the user when a filter is active.
(User-confirmed during brainstorming.)

**`status` is validated against the mailclassify vocabulary.** An unknown label
is a 400, mirroring the existing `inboxSources` guard — keeps the query from
silently returning empty on a typo.

## Risks / Trade-offs

- [No Deleted folder means deleted mail is only recoverable via the immediate
  Undo] → Acceptable: chosen deliberately; the row is soft-deleted (recoverable
  in the DB) even though the UI offers no browse-deleted view this iteration.
- [Mark-all-read is not itself undoable] → Low impact: marking read is
  low-stakes and reversible only per-message is fine; we do not add bulk-undo.
- [`unread`/`status` filters increase `ListEmails` WHERE complexity] → The new
  partial index on live rows plus the existing `(user_id, received_at)` index
  keep it cheap; filters are equality/`IS NULL`, sargable.

## Migration Plan

1. Apply migration `0022_emails_deleted_at.sql` (add column + partial index) —
   backward compatible; existing rows have `deleted_at = NULL` (live).
2. Deploy backend (new queries + endpoints) — old clients keep working; new
   params are optional.
3. Deploy web (toolbar controls + Delete/Undo).

Rollback: the column is additive and unused by old code; no data migration to
reverse.

## Open Questions

None — scope and the mark-all-read semantics were settled during brainstorming.
