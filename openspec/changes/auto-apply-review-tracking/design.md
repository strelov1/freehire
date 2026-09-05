## Context

`auto_apply_queue` (migrations/0116, /0128) already carries everything needed to derive a
richer status than the job-detail overlay does: `tailored_cv_id`, `review_decision`
(`approved`/`declined`/NULL), `blocked_at` (+ `unmapped` jsonb), `failed_at`, `last_error`. On
a successful submission the row is deleted in the same transaction that calls
`jobtracking.MarkJobApplied` (`cmd/auto-apply/store.go`), so a completed attempt leaves no
trace in `auto_apply_queue` — the only durable record is `application_events`. That call
stamps `EventSource: appevent.SourceSystem` (`"system"`), and grepping the module shows this
is, today, the *only* caller that writes an `applied` event with that source — every other
`applied` event comes from `SourceUser`, `SourceAssistant`, or a mail source. That makes
`event_type = 'applied' AND source = 'system'` a reliable, already-existing marker for "an
auto-apply attempt actually submitted this," with no schema change.

The existing review write path (`internal/api/handler/auto_apply_tailor.go`,
`POST /me/auto-apply/:queueId/review`) and its Inngest resume plumbing
(`auto_apply_review_publish.go`) are unchanged by this design — they already implement
approve/decline and resuming the orchestrator. This change only adds a way to read the
candidate's own attempts as a list and call that existing endpoint from a real page.

See proposal.md for motivation; see `specs/auto-apply-status-list/spec.md` for the behavior
contract this design implements.

## Goals / Non-Goals

**Goals:**
- One list endpoint that answers "what does my auto-apply queue look like right now, and
  what has it recently done" scoped to the caller.
- Reuse the existing review endpoint unchanged; this change only adds a reader and a page.
- No new database columns or migrations.

**Non-Goals:**
- No retry/unpark mechanism for a `blocked` or `failed` entry — none exists in the backend
  today (confirmed: no `Unpark`/requeue query or handler), and `auto_apply_queue`'s own
  `UNIQUE (user_id, job_id)` plus `PostJobAutoApply`'s permanent-decline check mean a
  blocked/failed/declined entry has no recovery path today regardless of what this page
  shows. Adding one is a separate, larger change (would need to relax the uniqueness
  constraint or add an explicit re-enqueue path) and is out of scope here.
- No pagination for either list in this iteration (bounded LIMIT is enough at current
  volume — a candidate's own queue is small); add it if usage shows otherwise.
- No change to the job-detail button's existing 4-state contract (`autoApplyButtonState`,
  `autoApplyEntryStatus`) — it keeps its own, coarser status derivation.

## Decisions

**One new handler, one new use case package function, two new sqlc queries.**
`internal/api/handler/auto_apply_list.go` mounts `GET /me/auto-apply` behind `mw.cookie`
(same auth posture as the enqueue/review endpoints — the browser is the only place a
candidate can watch and undo this). It calls a new function in
`internal/application/autoapply` (Fiber/pgx-free, per that package's existing convention)
that takes the two query results and derives the six-value status. Alternative considered:
compute the status entirely in SQL (a `CASE` expression per row) — rejected because the
status rules (e.g. "declined beats blocked even though decline also sets `blocked_at`") are
exactly the kind of business rule this package's Go layer already owns
(`autoApplyEntryStatus` in `auto_apply_enqueue.go` does the analogous 3-way derivation in Go,
not SQL), and keeping it in Go keeps one place testable without a live database.

**Two separate arrays in the response, not one merged/sorted timeline.**
`{"data": {"pending": [...], "recently_applied": [...]}}`. Alternative considered: merge both
into one chronological feed — rejected because the two sources answer different questions
("what needs me" vs "what already happened") and forcing one sort order across a queue table
and an event ledger adds a merge step for no reader benefit; the page renders them as two
sections either way (per the agreed frontend design).

**Recently-completed list reads `application_events`, not `user_jobs`.**
`user_jobs`/`applications` carries no per-row marker of *how* the candidate came to be marked
applied - only `application_events.source` does. Querying the ledger for
`event_type = 'applied' AND source = 'system'` (scoped to the caller, `ORDER BY occurred_at
DESC LIMIT 20`) is therefore the only correct way to isolate auto-apply's own submissions.
20 is a fixed cap for this iteration (see Non-Goals on pagination).

**`unmapped` is returned verbatim; `last_error` is never serialized onto any attempt.**
`unmapped` ([{id, label, required, reason}], migrations/0116) was designed to be legible
without replaying the attempt - it is exactly the "what got blocked" detail the candidate
asked for. `last_error` is an internal diagnostic string (e.g., a driver/HTTP error) never
intended for a candidate to read; the new use case's return struct simply has no field for it,
so there is no serialization path that could leak it by omission-of-a-filter.

**The job-detail button's own status function is left untouched.**
`autoApplyEntryStatus` (3 values: `queued`/`failed`/`declined`) has its own existing
consumers (`GetJob`'s overlay, `autoApplyButtonState` in the SPA) and its own doc comment
explaining why declined is checked before failed/blocked. The new 6-value derivation is a
separate function for a separate, richer surface - collapsing them into one shared function
now would force the button's simpler contract to either grow unused values or the list page
to lose the distinctions it needs.

## Risks / Trade-offs

- **[Risk]** A candidate reads "blocked" and expects a way to fix it, but there is no retry
  path → **Mitigation**: the page copy for `blocked`/`failed` should be explicit that the
  attempt is final for this job (matches `PostJobAutoApply`'s own existing "permanently
  stuck"/"already declined" wording), not implying a retry is possible.
- **[Risk]** `EventSource: appevent.SourceSystem` being auto-apply-exclusive is a convention,
  not an enforced invariant - a future system-initiated `applied` event from an unrelated
  feature would silently leak into "recently completed by auto-apply" → **Mitigation**: none
  needed structurally now (it is the only such caller today, and `appevent`'s own layering
  keeps the vocabulary in one file); worth a comment at the new query site cross-referencing
  this design so a future second `SourceSystem`-tagged `applied` event is a deliberate choice,
  not an accident.

## Migration Plan

No database migration. Additive-only: new queries, new handler, new route, new SPA page and
nav entry. Rollback is a plain revert - nothing else depends on the new endpoint or route.
