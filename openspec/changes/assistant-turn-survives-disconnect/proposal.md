## Why

A turn dies whenever a write to the client fails, and a phone backgrounding its
tab is exactly that: the work stops mid-run, minutes of model spend are thrown
away, and the user comes back to a chat that simply stopped. The observed cost is
a tailoring pass that made 25 CV edits and then lost its `tailor_report` to
`context canceled` — the report that tells the candidate what was covered.

The deeper fault is that one signal carries three meanings. A broken TCP write is
the only way the server learns that the user pressed Stop, that the user left, and
that the network blinked, so the code cannot tell a deliberate stop from a phone
locking its screen and must treat every one of them as "abandon the work".

## What Changes

- A failed SSE write no longer cancels the turn. The turn runs to completion under
  the bounds it already has — the server-owned step cap and the LLM call timeout —
  and the transcript it produces is persisted whether or not anyone was reading.
- Explicit cancellation gets its own channel: a new owner-scoped
  `POST /api/v1/assistant/sessions/:id/cancel`. The Stop button calls it instead of
  relying on aborting the stream. **BREAKING** for any client that relied on
  dropping the connection to stop a turn: dropping it now leaves the turn running.
- A message sent while a turn is in flight queues behind it instead of racing it.
  Capacity is one running turn plus one waiting per session; a further message is
  refused. The client is told it is queued rather than left guessing.
- On returning to a backgrounded tab the client re-reads the session and shows what
  the agent did while it was away. An interrupted stream stops being rendered as a
  failed turn, because a reader that stopped reading is not a turn that failed.
- The CV edit tool accepts a batch whose `ops` arrived as a JSON string, and applies
  a batch's removals in an order that does not invalidate its own addresses. Both
  are refusals that cost the model a round and taught it nothing about the CV.

## Capabilities

### New Capabilities

None. Every behaviour here belongs to a capability that already exists.

### Modified Capabilities

- `assistant-agent-runtime`: the turn survives a disconnected reader and stops only
  on explicit cancellation; concurrent messages to one session are serialised; the
  cancel channel is part of the contract rather than an accident of the transport.
- `cv-edit-revisions`: a batch is judged by what it asks for, not by how the model
  packaged it — argument packaging and intra-batch index shifts stop being refusals.

## Impact

- `internal/handler/assistant.go` — the write callback, the turn registry, the new
  cancel route, the queueing gate.
- `internal/handler/assistant_cv_tools.go` — `ops` unwrapping ahead of the strict
  decoder; `internal/cvedit` — removal ordering within a batch.
- `web/src/lib/assistant/` — `client.ts`, `AssistantChat.svelte`, `chat.ts`: Stop
  calls the endpoint, visibility restores the session, an aborted read is not an
  error.
- Strict decoding of unknown fields is deliberately untouched: it is what stopped an
  agent silently clobbering the wrong experience entry, and nothing here relaxes it.
- The registry lives in one process, so a turn that outlives a blue/green flip
  cannot be cancelled and instead ends at its step cap.
