## Decisions

**An agent may approve.** The person chose fully unattended operation. The guard is the
account-level `agents_may_apply` switch, off by default, which only the browser session can
turn on — an agent cannot grant itself the permission it is gated by. Per-grant OAuth scopes
would be finer; they are deferred until there is more than one kind of external client.

**Answers written through MCP are the candidate's.** The account server only ever acts for
the person who approved the connection; an answer it relays is theirs. Writing them as
`agent_inferred` would make them unsendable and stall every background attempt on the first
open question, which defeats the feature.

**History by trigger, not by each writer.** Four processes write `auto_apply_queue`
(the API, `cmd/auto-apply`'s two passes, the orchestrator through the API). A writer that
forgot to log would look exactly like nothing happening. A row-level AFTER trigger derives
the event from the OLD→NEW diff, so every present and future writer is covered. DELETE is
the exception: the trigger cannot tell a submission from a withdrawal, so the two deleting
queries insert their own event in the same statement.

Events carry no `last_error` text — it is an internal diagnostic the product never shows a
candidate (`autoapply.status.go`). The detail is structured: question labels for `blocked`,
the attempt number for `attempt_failed`/`captcha_refused`.

**Retry must never risk a second submission.** `deadLetterImmediately` covers two outcomes
where the employer may already hold the application. They are indistinguishable from an
ordinary exhausted budget by `failed_at` alone, so the worker now records them through
`DeadLetterUncertain`, which also stamps `submit_uncertain_at`. Retry refuses such a row.

**Cancel never races a submission.** Refused while the row is approved and its claim is
younger than the worker lease. The API reads the same `AUTO_APPLY_LEASE_SECONDS` default the
worker uses (`config.AutoApply`).

**Address by job slug.** A queue id dies on submit and cancel; the (user, job) pair is
already the queue's unique key and is what an agent holds after `search_jobs`.

**Shared use cases stay in `internal/api/handler`.** Enqueue needs the CV store, the plan
store, tracking, the eligibility gate and the Inngest publisher, all already on
`assistantHandlers`. Moving them into `internal/application` would mean re-plumbing five
dependencies for no second consumer outside this package; the transport-free method is the
seam, and both the route and the tool call it.
