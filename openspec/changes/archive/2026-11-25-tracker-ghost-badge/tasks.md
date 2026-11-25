## 1. Wire shape

- [x] 1.1 Add `Ghost *jobview.Ghost` to `jobview.Card` (`internal/job/jobview/card.go`), `json:"ghost,omitempty"`, mirroring `Job.Ghost`'s own doc comment (computed at read time, never stored).

## 2. Filter helper

- [x] 2.1 Add `In(attr string, ids []int64) string` to `internal/search/search/filter.go`, mirroring `NotIn` exactly (empty-slice → empty string, same numeric-id building).
- [x] 2.2 Unit test: `In` produces `attr IN [a, b, c]` for a non-empty slice and `""` for an empty one — mirror `NotIn`'s existing test(s).

## 3. Handler wiring

- [x] 3.1 Add a `queries *db.Queries` field to `trackingHandlers` (`internal/api/handler/user_jobs.go`); set it in `newTrackingHandlers` from the parameter already received.
- [x] 3.2 In `me_tracking.go`, add `attachGhostToTrackedCards(ctx context.Context, tracked []jobtracking.TrackedJob)` — a plain `context.Context`, not `*fiber.Ctx` (mirrors `ghostEvidenceFor`'s own signature choice; nothing here needs fiber, and it makes the method directly testable without HTTP plumbing), mutating `tracked[i].Job.Ghost` in place (same `*jobview.Card` pointer `items[i].Job` already copied, so no second pass over `items` is needed): collects ids from entries with a non-nil `Job`, no-ops when `h.search`/`h.queries` is nil or there are no ids, fetches stamps/evidence/reality class (via `ListJobGhostStamps`/`ghostEvidenceFor`/`search.In`-filtered `Search`) with the same `logGhostLookup` degrade discipline, then calls `jobview.ClassifyGhost` per tracked job.
- [x] 3.3 Call the new attach step from `ListTrackedJobs`, after the existing `items` loop, before the response is written.

## 4. Tests for the added spec requirements

- [x] 4.1 Scenario test: a tracked job whose absence stamp/evidence/reality class produce a non-`none` ghost level gets `ghost` set on its card. (`TestAttachGhostToTrackedCards/converged_criteria_produce_a_ghost_signal`)
- [x] 4.2 Scenario test: a tracked job with nothing to say (`ghost.LevelNone`) has `ghost` omitted (nil), same as today. (same subtest, `quietJobID`)
- [x] 4.3 Scenario test: an orphaned application (`Job == nil`) is skipped without panicking and without being included in the id batch sent to `search.In`/`ListJobGhostStamps`. (`.../nothing_panics_when_every_card_is_orphaned`, plus the filter-content assertion in the first subtest)
- [x] 4.4 Scenario test: `h.search == nil` leaves every card's `ghost` nil, and the response still succeeds. (`TestAttachGhostToTrackedCards_NilSearchIsANoOp`, plain unit test — no DB needed since the nil check is the first thing the method does)
- [x] 4.5 Scenario test: a `Search` error degrades to every card's `ghost` computed with `RealityClass = ""` rather than failing `ListTrackedJobs`. (`.../a_search_error_degrades_rather_than_failing`)
- [x] 4.6 Scenario test: a `Search` error does NOT suppress a signal whose non-reality criteria (ATS absence + user reports) already converge on their own — proves the degrade narrows evidence rather than blanket-omitting `ghost`. Found on review (CodeRabbit): the spec's original wording implied every lookup failure omits `ghost` unconditionally, which was inaccurate for this case. (`.../evidence-only_criteria_still_converge_when_Search_fails`)
- [x] 4.6 Run `go vet -tags=integration ./...` and the full test suite for `internal/job/jobview`, `internal/search/search`, and `internal/api/handler`; run `TestMeasureBoardLoad` (`-tags=integration`) and confirm the payload-ceiling assertion still passes.

## 5. Wrap-up

- [x] 5.1 Update `internal/application/userjob/AGENTS.md`'s note ("Two read-time signals are absent from the card... Reality and Ghost are attached by explicit calls the tracking path has never made") to reflect that Ghost is now attached, while Reality remains a detail-page-only signal by existing convention (not this change's gap to close).
- [x] 5.2 `gofmt -l .`, `go vet ./...`, `go test ./...` clean before commit.
