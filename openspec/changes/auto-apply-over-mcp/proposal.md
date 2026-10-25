## Why

Auto-apply runs end to end, but only from a browser: the enqueue route is cookie-only, the
review step is reachable from the drawer, and nothing else can see or steer an attempt. An
agent connected to the signed-in MCP server (`/api/v1/mcp/account`) can find a job and
save it, but cannot apply to it, cannot tell what happened to an attempt, and cannot answer
the question an attempt stopped on. Three gaps also make the background flow brittle for
anyone, browser or agent:

- a parked (`blocked`) or dead-lettered attempt is terminal — answering the missing question
  later changes nothing, and re-enqueueing is refused because the row still exists;
- an attempt cannot be withdrawn;
- only the latest state is stored (`last_error`, counters), so "what happened, in order" is
  not answerable at all.

## What Changes

- **Event history.** New table `auto_apply_events`, written by a trigger on
  `auto_apply_queue` for every state transition (queued, tailored, ready for review,
  approved, declined, blocked with its questions, attempt failed, captcha refused, failed,
  retried) plus explicit writes for the two deletes (submitted, cancelled). Keyed by
  `(user_id, job_id)` so the timeline survives the row's deletion and a re-enqueue.
- **Retry.** Re-arms a blocked or failed attempt: clears the park/dead-letter markers and
  resets the budget that ran out; a tailoring failure re-publishes `auto-apply/submit` so
  the orchestrator tailors again. Refused for a decline and for an attempt whose submission
  may already have reached the employer (new `submit_uncertain_at`, set by the worker's
  immediate dead-letter path) — retrying those risks a duplicate application.
- **Cancel.** Deletes an attempt that is not mid-submission and records `cancelled`.
- **Agent permission.** New `users.agents_may_apply` (default false), toggled on the
  Connected devices page via a cookie-only route. The MCP tools that start, review, retry or
  cancel an attempt refuse while it is off. Reading is always allowed.
- **MCP tools on the account server:** `auto_apply_start`, `auto_apply_list`,
  `auto_apply_get` (status, open questions, timeline), `auto_apply_review`,
  `auto_apply_retry`, `auto_apply_cancel`, `answer_bank_list`, `answer_bank_save`,
  `answer_bank_delete`, `screening_answers_get`, `screening_answers_set`. All address an
  attempt by job slug. An answer saved through MCP is the candidate's own
  (`answerbank.AuthorCandidate`) — the agent is relaying the person it is connected for.
- **HTTP parity:** `POST /me/auto-apply/:queueId/retry` and `DELETE /me/auto-apply/:queueId`,
  plus Retry / Cancel buttons in the tracker drawer's auto-apply section.
- Enqueue and review logic move out of the Fiber handlers into transport-free methods both
  the HTTP routes and the MCP tools call.

## Impact

- Migration `0196_auto_apply_events.sql`.
- `internal/api/handler` (auto-apply enqueue/review/retry/cancel, MCP tool set, agent
  permission route), `internal/application/autoapply` (Store gains `DeadLetterUncertain`),
  `cmd/auto-apply` store, `internal/api/mcpapp` instructions, web Connected devices view and
  JobDrawer.
- Out of scope: native submission for Ashby/Workable/Lever, per-grant OAuth scopes,
  live verification of the orchestrator on the host (tracked in
  `auto-apply-inngest-orchestration` 5.5b).
