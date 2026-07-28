## 1. The silence vocabulary and thresholds

- [x] 1.1 Add the stage→threshold table to `internal/userjob`, alongside the existing stage vocabulary in `stages.go`, with each value's provenance recorded at the point of definition (measured / interpolated / judgement)
- [x] 1.2 Add the silence-state vocabulary (`active`, `silent`, `unconfirmed`) and the pure function mapping (stage, days silent, has-pending-suggestion) to a state
- [x] 1.3 Cover the pure mapping: below/at/past each threshold, the same silence judged differently by stage, unset stage judged as `applied`, and terminal stages yielding no state at all
- [x] 1.4 Cover the precedence rule: a pending suggestion turns what would be `silent` into `unconfirmed`, but never turns `active` into anything

## 2. Deriving last activity

- [x] 2.1 Extend the `/me/tracking` query with the last-activity aggregate — `GREATEST(applied_at, max(received_at))` over linked, non-deleted mail — and a flag for whether any unconfirmed suggestion points at the application
- [x] 2.2 Cover the aggregate against a real Postgres: no linked mail falls back to `applied_at`; linked mail moves it forward; another application's mail is ignored; an unconfirmed suggestion is not activity; soft-deleted mail is excluded
- [x] 2.3 Confirmed against prod: `EXPLAIN (ANALYZE)` on the last-activity aggregate takes a `Bitmap Index Scan on emails_job_id_idx`, 16ms on the busiest application. The existing partial index serves it; no index added

## 3. Wire shape

- [ ] 3.1 Add `last_activity_at`, `days_silent` and `silence_state` to the `/me/tracking` row, null on any row that is not an application
- [ ] 3.2 Cover the wire shape: an application carries all three; a viewed-or-saved-only row carries none
- [ ] 3.3 Regenerate the API contract and check the new fields appear
- [ ] 3.4 Update `internal/userjob/AGENTS.md` and `internal/handler/AGENTS.md`

## 4. Tracking board

- [ ] 4.1 Render the silence marker on the application card: days silent when `silent`, an invitation to confirm the pending mail when `unconfirmed`, nothing when `active` or terminal
- [ ] 4.2 Verify visually that a board of mixed states reads correctly, and that a board with no silent applications shows no markers at all
- [ ] 4.3 Confirm the `unconfirmed` card links to the mail awaiting confirmation, so the question it asks can be answered in one step

## 5. Verification

- [ ] 5.1 `gofmt`, `go build ./...`, `go vet ./...`, `go test ./...`, and the integration suites for `./internal/db/` and `./internal/handler/`
- [ ] 5.2 Compare the live account's flagged applications against the measurement in the proposal (15 of 92 at `applied`, 3 of 6 at `interview`); an unexplained divergence means the implementation and the measurement disagree about what silence is
