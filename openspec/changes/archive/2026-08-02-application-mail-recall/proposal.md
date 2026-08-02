## Why

Linking mail to an application only ever runs one way. A message arrives, the
classification worker asks which application it belongs to, and either links it or
files a suggestion. Nothing asks the opposite question. Every surface starts from a
message — the suggested queue, the unlinked queue, manual linking — so a candidate
looking at an application that plainly ought to have mail, and does not, has nowhere
to ask for it.

The calendar inherits the same gap and pays more for it. A meeting is attached to an
application only through the `iCalUID` of an invitation the mail matcher already
linked. When that link never happened, the sync reads the meeting, matches nothing,
and discards it — the interview is invisible, and no row records that it was ever
seen.

## What Changes

- A **new action on an application**: sweep the caller's mailbox for messages
  belonging to that application and propose them.
- The sweep is a **single batched model call**, not an agent turn: a deterministic
  net gathers candidates by link state and a time window, one call adjudicates them,
  and the confident ones are written as that application's pending suggestion.
- Confirmation reuses the **existing** confirm/reject on a suggestion. No new
  confirmation surface, and the rule that only a deterministic tier may link is
  untouched — a model's pick stays a suggestion.
- The action can only **add**: it never links, never moves a stage, never writes to
  the event ledger, and cannot touch a message already linked to an application.
- No calendar code. An invitation linked by this action produces its meeting on the
  next calendar sync, which re-reads its whole window every run.
- The Emails tab of the application drawer gains the button and renders the result
  with the row it already draws.

No breaking changes.

## Capabilities

### New Capabilities
- `application-mail-recall`: from an application, find the caller's mail that belongs
  to it and offer it as suggestions — the pull direction of mail-to-application
  linking, its bounds, its spend attribution, and what it is forbidden to do.

### Modified Capabilities
<!-- None. Confirming a suggestion, linking, stage advancement and the ledger are
     all unchanged; this change only produces suggestions the existing surfaces
     already resolve. -->

## Impact

- **New**: `internal/mailrecall` (the net, the adjudication, the bounds).
- **API**: `POST /api/v1/me/tracking/:slug/mail-recall`, under a session cookie or a
  full-scope key, beside the existing follow-up endpoints on the same prefix.
- **SQL**: two queries in `internal/db/queries/mail_linking.sql`, then `make sqlc`.
  **No migration** — the change writes to columns that already exist.
- **Web**: the button and result list in `web/src/lib/components/JobDrawer.svelte`;
  types through `cmd/gen-contracts`.
- **Spend**: one call per press on the caller's own gateway credential
  (`internal/llmkey`), tagged `feature:mail-recall`. No credit debit.
- **Unchanged**: `internal/maillink`, `internal/mailmatch`, `internal/mailclassify`,
  `internal/calmatch`, `internal/calsync`, and every existing mail endpoint.
