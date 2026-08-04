## Context

`streamTurn` (`internal/handler/assistant.go:441`) already runs the turn on
`context.Background()`, deliberately, because the request context dies when the
handler returns and the stream writer outlives it. What kills the turn is one
line inside the event callback: when `stream.event` reports a failed write, the
handler calls `cancel()`.

That was reasonable when the only client was a desktop tab, and it is wrong on a
phone. A backgrounded tab is frozen, not closed, and the failed write it produces
is indistinguishable from the user closing the app. It is also indistinguishable
from the Stop button, because Stop is `controller.abort()`
(`web/src/lib/assistant/client.ts:104`) — the client has no other way to say
"stop" and the server has no other way to hear it.

Two bounds on a turn already exist and are server-owned: `RunnerConfig.MaxSteps`
(8, raised to 30 for an autopilot run) and the LLM client's per-call timeout.
Cancellation-on-disconnect was a third, cruder bound layered on top.

Separately, the tool contract refuses batches for reasons that carry no
information about the CV: 15 refusals in 30 days because `ops` arrived as a
stringified array, and a refusal when a batch removes two positions of one list.
Each refusal costs a round of the very budget the run needs to finish.

## Goals / Non-Goals

**Goals:**

- A turn survives a reader that stops reading, and finishes under its existing bounds.
- Stopping a turn becomes an explicit request rather than a side effect of transport.
- Two turns never run against one session, and therefore never against one CV.
- A returning client sees the work done in its absence.
- Refusals that are about packaging rather than intent stop happening.

**Non-Goals:**

- Moving autopilot to a queue and a worker. That is the right end state for an
  unattended run and it is a larger change; this one keeps the turn in-process.
- Surviving a process restart or a blue/green flip. A turn is process-local
  before this change and stays so after it.
- Relaxing strict decoding of unknown fields anywhere.
- Metrics or alerting on tool failures. Worth doing, separate change.

## Decisions

**A failed write stops writing, not working.** The event callback drops the
`cancel()` and simply stops attempting to write. The alternative — a grace period
before cancelling — was considered and rejected: an autopilot run takes minutes,
so any grace short enough to save money is too short to save the run, which is
the case that hurt.

**Cancellation gets a registry and an endpoint.** `assistantHandlers` holds
`map[sessionID]*turnState` under a mutex; `POST /sessions/:id/cancel` looks up
the session's turn and cancels it. The registry is chosen over storing a
cancellation flag in Postgres because a `CancelFunc` cannot be persisted and the
handler that must act on it lives in this process. Consequence, accepted and
recorded in the proposal: a turn started before a blue/green flip cannot be
cancelled and ends at its step cap.

**Owner scoping reuses the existing rule.** Cancel resolves the session with the
same owner-scoped read as everything else, so a session the caller does not own
is reported missing rather than forbidden.

**Queueing is a per-session slot, not a queue.** The registry entry carries a
channel closed when the turn ends; a second message waits on it with a timeout,
a third is refused with 409. A real queue (durable, ordered, many-deep) buys
nothing here: the waiting message is a live HTTP request, so its natural depth is
one.

**The `queued` event precedes the turn's own events.** The client already renders
named events, so telling it "you are waiting" needs no new transport.

**`ops` is unwrapped at the tool boundary, not inside `cvedit`.** The tool is
where a model's packaging guess arrives; `cvedit` should keep receiving typed
operations. Unwrapping is: if `ops` decodes as a string, decode that string as
the array and proceed through the same strict decoder.

**Removals within a batch are applied in descending index order.** The batch is
already all-or-nothing, so reordering the application of removals changes no
outcome except the self-inflicted refusal. Sorting happens where the batch is
applied, so every entry point inherits it.

## Risks / Trade-offs

- **An abandoned turn now costs full budget** → It was already bounded by
  `MaxSteps` and the call timeout; the worst case is one abandoned autopilot run
  at 30 rounds, and that is the exact scenario the user asked to protect.
- **A turn cannot be cancelled across a deploy flip** → It ends at its step cap;
  documented in the proposal rather than solved, because solving it means moving
  runs out of process.
- **The registry leaks if an entry is not removed** → The entry is removed in a
  `defer` alongside the existing `cancel` defer, and a test asserts the map is
  empty after a turn ends, including a failed one.
- **Unwrapping `ops` could mask a genuinely malformed batch** → The unwrapped
  string goes through the same strict decoder, so a malformed array still fails
  with the same message; only the wrapper is forgiven.
- **Two clients on one session both press Stop** → Cancelling an already-finished
  turn succeeds and does nothing, per the spec.

## Migration Plan

No schema change, no data migration. The change is a deploy; the frontend and
backend ship together because Stop moves to the new endpoint. A client running
the old bundle against the new backend still works — its abort stops its own
reading, and the turn it abandoned finishes and is stored rather than truncated.

Rollback is a redeploy of the previous build: nothing persists that the old code
cannot read.

## Open Questions

None outstanding. The wait timeout for a queued message and the step-cap
interaction are implementation constants, chosen in the tasks and testable.
