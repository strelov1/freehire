## Why

The inbox is the one feature a user cannot self-serve. Mail reaches freehire only
through a Gmail OAuth connection we must get verified for a restricted scope, or
through a hosted mailbox we pay SES to receive; the classification that makes the
mail useful runs on an LLM we pay for. Every new inbox user therefore costs money
before earning any.

Users who run their own agent harness already have the two expensive pieces:
a mail client (`himalaya`, `mbsync`, anything IMAP) and an LLM. What they lack is
a way in. Opening the inbox as a key-authenticated CRUD surface lets such a user
bring their own transport and their own classifier, and get the tracker
integration for free — the free tier that costs us nothing but the API surface.

## What Changes

- The mail surface (`/me/inbox`, `/me/emails/*`, `/me/mailbox`, `/me/gmail`
  status) moves from cookie-only auth to cookie-or-full-scope-key
  (`RequireAuthOrKey`), matching the tracker endpoints an agent already drives.
- New `POST /api/v1/me/emails` ingests a batch of messages the caller's own
  harness fetched, stored under `source = 'external'` and deduplicated on
  `(user_id, source, external_id)` so a re-sync is idempotent.
- New `POST /api/v1/me/emails/:id/triage` writes one agent-produced verdict —
  status signal, optional application link, optional confidence — in a single
  update, then advances the application stage through the existing
  `mailclassify.AdvanceStage` rules. It mirrors what the LLM worker writes.
- `GET /api/v1/me/inbox` gains `?body=1` (full bodies inline, so an agent triages
  a page without an N+1 of `GET /me/emails/:id`, and without that endpoint's
  mark-as-read side effect) and `?unclassified=1` (the agent's work queue).
  `?source=external` joins the account-switcher vocabulary.
- The server never classifies `source = 'external'` mail: the pending-classification
  sweep excludes it, so a self-hosted user's mail costs no LLM tokens.
- `freehire-cli` gains an `inbox` command group — list, read, push, triage, link,
  unlink, read-all, delete/restore — and its agent skill documents the sync loop.
- Existing Gmail and hosted-mailbox ingestion is untouched and keeps classifying
  server-side. This is a third path, not a replacement.
- Fetching mail (IMAP, Gmail API, OAuth) stays **out of scope**: the user's harness
  owns the transport, the CLI only accepts what it hands over.

## Capabilities

### New Capabilities

- `agent-inbox-surface`: the key-authenticated agent contract over the inbox —
  batch ingest of externally-fetched mail, the single-call triage write, the
  agent-shaped listing (inline bodies, unclassified work queue), and the rule that
  external mail is never server-classified.

### Modified Capabilities

None. Two neighbouring specs were checked and neither changes at the requirement
level:

- `email-inbox` states its requirements for "an authenticated user" without naming
  the credential, so widening the accepted credential adds a requirement rather
  than changing one — it is stated in the new capability instead.
- `api-keys` already confines a `cv`-scoped key to "the CV endpoints under
  `/api/v1/me/cvs` plus the caller's own identity read", which refuses it on the
  mail surface with no edit.

## Impact

**Backend** — `internal/handler/handler.go` (route auth for the mail block),
`internal/handler/inbox.go` (filters, listing shape), a new
`internal/handler/inbox_agent.go` (ingest + triage), `internal/db/queries/gmail.sql`
(listing filters, external upsert), `internal/db/queries/mail_classification.sql`
(exclude external from the pending sweep, agent triage write), regenerated
`internal/db`. Reuses `internal/mailclassify` unchanged.

**No migration.** `emails.source` is plain `TEXT` with no check constraint, and the
unique key is already `(user_id, source, external_id)`; `'external'` needs only code.

**CLI** — `../freehire-cli` (separate repo, no OpenSpec): `internal/cli/inbox.go`,
`internal/client/inbox.go`, `skills/using-freehire/SKILL.md`, README.

**Web** — the source switcher gains an `external` option; the inbox otherwise
renders agent-triaged mail with no change, since it reads the same columns.
