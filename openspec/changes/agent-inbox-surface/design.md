## Context

The inbox already has everything except a way in for an agent. One table
(`emails`) holds mail from two sources discriminated by `emails.source`, unique on
`(user_id, source, external_id)`. A classification overlay (`status_signal`,
`job_id`, `suggested_job_id`, `link_source`, `match_confidence`, `classified_at`,
`classification_model`) is written by `cmd/classify-mail` draining
`email_classification_outbox`, and read by both `/my/inbox` and the tracker's
application detail.

Two things block a CLI agent today. Every mail route registered by
`inboxHandlers.register` (`internal/handler/gmail.go`) is mounted on `mw.cookie`
— cookie only, so a `Bearer fhk_…` request is a 401 — while the neighbouring
tracker routes (`/jobs/:slug/apply`, `/track`) already mount `mw.key`
(`RequireAuthOrKey`). And `status_signal` has exactly one writer,
`SetEmailClassification`, reachable only from the worker; from outside, an agent
can link a message but cannot say what it is.

The handler package was split into per-feature handler structs in #1158, so this
change lands as edits to one cohesive unit — `inboxHandlers` and its files
(`gmail.go`, `inbox.go`, `inbox_linking.go`, `mailbox.go`) — rather than as
scattered additions to a monolithic router.

`internal/mailingest` already frames ingestion as "the transport abstraction the
worker consumes… swappable (SES today)". This change does not add a transport to
that interface — it moves the boundary outward, making the caller's harness the
source over HTTP.

## Goals / Non-Goals

**Goals:**

- A user running their own agent harness can push mail they fetched themselves,
  read it back, classify it, and link it to tracked applications — entirely
  through `freehire-cli` with one full-scope API key.
- Agent-produced classifications are indistinguishable, to every reader, from
  worker-produced ones: same columns, same stage-advance rules, same web UI.
- Self-hosted mail costs zero LLM tokens.
- No regression for Gmail and hosted-mailbox users.

**Non-Goals:**

- Fetching mail. No IMAP client, no OAuth, no polling — the harness owns it.
- A pull-based work-queue protocol (claim/lease/ack). One user with one harness
  has no concurrency to arbitrate; the seam is noted, not built.
- Server-side classification of external mail, even as an opt-in. If that becomes
  wanted it is a credits-debiting endpoint, and a separate change.
- A narrow `mail` API-key scope. Deliberately deferred; see Risks.

## Decisions

### One triage call, not per-field CRUD

`POST /me/emails/:id/triage {signal, slug?, confidence?}` performs a single
`UPDATE` setting `status_signal`, `job_id`, `link_source='agent'`,
`match_confidence`, `suggested_job_id=NULL`, `classification_model='agent'` and
`classified_at=now()`, then runs `mailclassify.AdvanceStage`.

*Alternative considered — separate `/classify` and reuse of `/link`.* Rejected
because it manufactures states the worker never produces: classified but unstamped,
or linked but unclassified. `SetEmailClassification` writes all of it at once
precisely because it is one verdict; a second writer of the same verdict should
write it the same way.

*Alternative considered — expose `email_classification_outbox` as claim/results
endpoints.* Rejected as infrastructure ahead of need: leases and dead-lettering
exist to arbitrate competing workers, and a single user's harness has no
competitor. `?unclassified=1` gives the same "find my work" affordance for free.

The existing `/link`, `/unlink`, `/confirm`, `/reject` endpoints are untouched.
They are the human's manual corrections and stay exactly as they are.

### `link_source='agent'`, not `'manual'`

Provenance is worth keeping: `auto` (matcher), `manual` (a person clicked),
`agent` (the caller's harness decided). Nothing renders `link_source` as a label
today — `web/src/lib/types.ts` carries it and `InboxView.svelte` passes it through
— so a third value is additive. `classification_model` records `agent` for the
same reason: it is the "which model produced this" column, and the honest answer
is "not ours".

### The pending sweep skips external mail

`EnqueuePendingEmailClassification` currently selects every email with
`classified_at IS NULL`. It gains `AND source <> 'external'`. This is the single
line that makes the free tier free: without it, every pushed message would be
picked up by `cmd/classify-mail` on its next run and billed to us.

It also means external mail can sit unclassified forever, which is correct — its
classifier is the caller's, and `?unclassified=1` is how they find the backlog.

### Bodies in the listing, not an N+1 of message reads

`GET /me/inbox?body=1` returns each message's readable body inline. Two reasons,
one obvious and one not.

The obvious one is the round-trip count, and there is a precedent in this codebase:
`AgentSearchJobs` "replaces the index's truncated preview with each job's full
description — so a caller reads a result set without a follow-up GetJob per hit."

The non-obvious one is that `GET /me/emails/:id` marks the message read
(`MarkEmailRead` in `inboxHandlers.GetEmail`). That side effect is right for a person opening mail and wrong
for an agent sweeping it: `read_at` means "a human has seen this", and an agent
that triages the backlog would silently zero the user's unread count. The listing
has no such side effect, so routing the agent through it preserves the signal.

The body returned is the *readable* body — plain text when present, HTML stripped
to text otherwise. `maillink.readableBody` already implements exactly this rule
and gets exported as `ReadableBody` so the handler and the worker cannot drift.

Implementation note: `ListEmails` already detoasts `body_text` to build the
160-character snippet, so selecting it costs no extra read. The query gains
`CASE WHEN sqlc.arg(with_body)::bool THEN emails.body_text ELSE '' END` plus
`body_html` under the same guard, rather than a duplicated second query — the
`WHERE` clause is long and copying it would be the real maintenance cost.

### Ingest reports inserted vs updated

`UpsertExternalEmail` uses `ON CONFLICT (user_id, source, external_id) DO UPDATE`
and returns `(xmax = 0) AS inserted` to distinguish a first push from a re-push.
It updates only the content columns (`thread_id`, `from_addr`, `from_name`,
`subject`, `body_text`, `body_html`, `received_at`) — never `read_at`,
`deleted_at`, or any classification column, so a re-sync cannot resurrect a
deleted message, un-read a read one, or wipe a triage verdict.

Batches are capped at 100 messages, rejected rather than truncated. A whole batch
is written in one transaction (`pool.Begin` + `queries.WithTx`, the pattern
`jobtracking.MarkApplied` uses) so a partial batch is never half-stored;
`inboxHandlers` gains the pool it needs for that.

### Full-scope key, no new scope

The mail routes move from `mw.cookie` to `mw.key`. A `cv`-scoped key is already
confined by `mw.cvKey` to the CV surface, and `mw.key` is documented in
`middleware` as "full-scope-only — so a new endpoint is out of a leaked agent
credential's reach unless it opts in", so a `cv` key stays refused with no edit.

*Alternative considered — a narrow `mail` scope.* Mail is the most sensitive data
in the product, so a dedicated scope is attractive. It was rejected for now
because the agent must also reach the tracker to link and to read applications; a
`mail`-only key would force the harness to hold two credentials to do one job,
which is worse security in practice than one key the user can revoke.

## Risks / Trade-offs

**A leaked full-scope key reads all the owner's mail** → Keys are already
revocable and hashed at rest, and the same key already reaches the CV and the
tracker. The mitigation is the existing revoke flow, plus documenting in the CLI
skill that the key belongs in the harness's secret store, not in a repo. Revisit
a `mail` scope if key-sharing patterns emerge.

**An agent writes a wrong signal and advances a stage incorrectly** → Advancement
runs through `mailclassify.AdvanceStage` unchanged, which only moves forward and
refuses to resurrect a settled application — the exact guard added after the
`rejected → applied` incident on prod. Triage is also idempotent and re-runnable,
and the user can still correct a stage by hand.

**Prompt injection through pushed mail** → Bodies are untrusted text and this
change does not feed them to any LLM of ours; the risk moves to the caller's
harness, where it is theirs to manage. The CLI skill says so plainly.

**Stripping HTML for a page of messages costs CPU on the read path** → The agent
listing is capped at 50 per page (below the general clamp) and is low-traffic.
Bodies are only stripped when `body=1` is asked for, so the web inbox is unaffected.

**Storage growth from users pushing their whole mailbox** → The batch cap slows it
but does not bound it. Not solved here; flagged as the first thing to watch if the
free tier is adopted. The existing soft-delete and mailbox-release paths already
give a way to purge.

## Migration Plan

No schema migration. `emails.source` is plain `TEXT` with no check constraint and
the unique key already includes it, so `'external'` is a code-only value.

Deploy order is unconstrained: the query change to
`EnqueuePendingEmailClassification` is additive-restrictive and safe to ship before
any external mail exists. Rollback is a plain revert; already-pushed external mail
stays readable in the web inbox, and would simply start being enqueued for
server-side classification again — the only behavioural consequence of a rollback,
and worth knowing before rolling back.

The CLI ships from its own repo on its own cadence; it degrades to a clear API
error against an older server.

## Open Questions

None blocking. Deferred by decision: a narrow `mail` key scope, an opt-in
credits-debiting server classification for external mail, and a claim/lease
protocol if multi-harness use ever appears.
