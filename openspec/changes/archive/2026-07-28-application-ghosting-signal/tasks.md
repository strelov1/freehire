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

- [x] 3.1 Add `last_activity_at`, `days_silent` and `silence_state` to the `/me/tracking` row, null on any row that is not an application
- [x] 3.2 Cover the wire shape: an application carries all three; a viewed-or-saved-only row carries none
- [x] 3.3 Regenerated the API contract: no diff — `MyJob` in `web/src/lib/types.ts` is hand-maintained rather than generated, so the three fields were added there directly
- [x] 3.4 Update `internal/userjob/AGENTS.md` and `internal/handler/AGENTS.md`

## 4. Tracking board

- [x] 4.1 Render the silence marker on the application card: days silent when `silent`, an invitation to confirm the pending mail when `unconfirmed`, nothing when `active` or terminal
- [ ] 4.2 **Not done — blocked.** Visual verification of a mixed-state board needs the API deployed; prod does not serve these fields yet. Only the absent-fields case is provable by reading: `silence_state` is `undefined` there, every branch tests `=== 'silent'` / `=== 'unconfirmed'`, so no marker renders and nothing dereferences `days_silent`
- [ ] 4.3 **Not done.** The `unconfirmed` marker explains itself in a tooltip but is not a link; answering the question still means opening the card and finding the mail. Worth doing, and deliberately not claimed

## 5. Verification

- [x] 5.1 `gofmt`, `go build ./...`, `go vet ./...`, `go test ./...`, and the integration suites for `./internal/db/` and `./internal/handler/`
- [x] 5.2 Verified against prod with the shipped ladder: 15 of 92 at `applied`, 3 of 6 at `interview`, 0 of 1 at `screening` — matching the proposal exactly, using the same floor-to-whole-days rule the code applies
