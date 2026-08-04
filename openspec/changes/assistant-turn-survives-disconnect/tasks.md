## 1. Tool contract: stop refusing batches over packaging

- [x] 1.1 Failing test in `internal/cvedit`: a batch removing positions 3 and 4 of a four-element list currently refuses itself; assert both elements go and the document is left consistent
- [x] 1.2 Apply a batch's `remove` operations in descending index order at the point the batch is applied, so every entry point inherits it; keep the batch all-or-nothing
- [x] 1.3 Failing test in `internal/cvedit`: a batch naming a position the list never held is still refused and leaves the document unchanged
- [x] 1.4 Failing test in `internal/handler`: `cv_edit` called with `ops` as a string holding a JSON array is refused today; assert it applies identically to the array form
- [x] 1.5 Unwrap a string-valued `ops` at the tool boundary in `assistant_cv_tools.go`, then decode through the existing strict decoder unchanged
- [x] 1.6 Failing test: an operation carrying an unknown field is still refused and names the field — the 2026-07-19 guard must survive this change
- [x] 1.7 Return an addressed message when `evidence_id` names no banked achievement, telling the model to take the id from `experience_search`

## 2. Turn registry

- [x] 2.1 Failing test in `internal/handler`: two turns started for one session both run today; assert the registry admits one and reports the session busy to the second
- [x] 2.2 Add the per-session turn registry to `assistantHandlers`: session id to turn state holding its `CancelFunc` and a channel closed when the turn ends, guarded by a mutex
- [x] 2.3 Register a turn as it starts and remove it in a `defer` beside the existing `cancel` defer
- [x] 2.4 Failing test: the registry is empty after a turn ends, including a turn that ended in error

## 3. The turn stops depending on its reader

- [x] 3.1 Failing test in `internal/handler`: with every SSE write failing from the first event, the turn is cut short today; assert it runs to its own end and its messages are persisted
- [x] 3.2 Remove the `cancel()` call from the SSE event callback in `streamTurn`; a failed write stops writing only
- [x] 3.3 Failing test: a turn whose reader vanished mid-run still reaches and stores its final tool call, mirroring the lost `tailor_report`
- [x] 3.4 Confirm the keep-alive goroutine and the write deadline still end with the turn and leak nothing when nobody reads

## 4. Cancellation gets its own channel

- [x] 4.1 Failing test: `POST /api/v1/assistant/sessions/:id/cancel` does not exist; assert it stops the session's running turn before its next model call
- [x] 4.2 Add the route and handler, resolving the session with the same owner-scoped read used elsewhere
- [x] 4.3 Failing test: cancelling a session with no turn in flight succeeds and changes nothing
- [x] 4.4 Failing test: a caller who does not own the session gets the session reported as missing, and the turn keeps running
- [x] 4.5 Failing test: work committed before cancellation stays committed and the partial transcript is stored

## 5. One turn at a time within a session

- [x] 5.1 Failing test: a message arriving during a running turn waits for it and then runs as its own turn
- [x] 5.2 Make a second message wait on the running turn's completion channel, bounded by a timeout
- [x] 5.3 Failing test: a third message, arriving while one waits, is refused with 409 and disturbs neither the running nor the waiting turn
- [x] 5.4 Emit a `queued` event to the waiting client before its turn's own events
- [x] 5.5 Failing test: a queued message whose wait times out fails cleanly with a terminal event rather than hanging the client

## 6. The client stops lying about interrupted turns

- [x] 6.1 Failing test in `web`: an aborted read currently marks the message errored; assert an interrupted stream is not rendered as a failed turn
- [x] 6.2 Point the Stop button at the cancel endpoint; keep `abort()` as the way this client stops reading
- [x] 6.3 Re-read the session on `visibilitychange` when a turn was in flight, and render the transcript the server holds
- [x] 6.4 Render the `queued` event so a waiting message reads as waiting rather than as a stalled turn
- [ ] 6.5 Failing test: returning to a tab whose stream was interrupted shows the messages produced in the meantime

## 7. Verification

- [x] 7.1 `go build ./... && go vet ./...` and `go test ./...`
- [x] 7.2 `go vet -tags=integration ./...` — the cheap guard against a changed signature breaking the tagged suite CI runs
- [x] 7.3 `go test -tags=integration ./internal/handler/` for the turn and tool tests
- [x] 7.4 `pnpm test`, `pnpm lint` and `pnpm check` in `web/`
- [ ] 7.5 Drive the real flow: start an autopilot run, background the tab for a minute, return, and confirm the run finished and filed its report
