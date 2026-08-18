## 1. SQL: batch-fetch excluded skills by user id

- [x] 1.1 Add `ListUserProfilesExcludedSkills` (or similarly named) query to `internal/db/queries/user_profiles.sql`: `SELECT user_id, excluded_skills FROM user_profiles WHERE user_id = ANY(sqlc.arg(user_ids)::bigint[])`.
- [x] 1.2 Run `make sqlc` and confirm the generated code compiles (`go build ./...`).

## 2. notify: Store wiring for the new query

- [x] 2.1 Add `ListUserProfilesExcludedSkills(ctx context.Context, userIDs []int64) ([]db.ListUserProfilesExcludedSkillsRow, error)` to the `internal/notify.Store` interface (in `internal/notify/notify.go`) — `*db.Queries` already satisfies it via the generated method from task 1.1/1.2, so no adapter type is needed.
- [x] 2.2 Add the corresponding method to `internal/notify`'s test `fakeStore` (in `internal/notify/notify_test.go`), backed by an in-memory `map[int64][]string` the tests can seed directly.

## 3. notify: per-subscriber avoid-skills exclusion in matching

- [x] 3.1 In `internal/notify/match.go`'s `Runner.match`, after `ListActiveSubscriptions`, collect the distinct `user_id`s across all active subscriptions and call `Store.ListUserProfilesExcludedSkills` once to build a `map[int64][]string`.
- [x] 3.2 Thread that map into `matchQuery` (new parameter) and, in its `for _, hit := range res.Hits { for _, s := range subs {...} }` loop, skip recording `(s.ID, hit.ID)` when `hit.Skills` intersects `excludedByUser[s.UserID]` (case-insensitive set membership — skills are already canonical/lowercased from the dictionary, so a direct string compare is sufficient; confirm against `internal/skilltag` normalization before assuming no extra normalization step is needed).
- [x] 3.3 Update `internal/notify/match.go`'s doc comment (lines 14-21) to describe the new avoid-skills gate alongside the existing `start_at` gate.

## 4. Tests: internal/notify

- [x] 4.1 Write a failing test in `internal/notify` (fake store/searcher, following the existing pattern in `internal/notify/notify_test.go`) for: a job matching the shared query but carrying a skill in one subscriber's `excluded_skills` is not recorded as a match for that subscriber.
- [x] 4.2 Extend/add a test for: two subscriptions sharing one canonical query, only one subscriber has the matching skill excluded — the job is recorded for the other subscriber and not for the excluding one (verifies the fan-out stays per-subscriber, not per-query).
- [x] 4.3 Add a test for: a subscriber's `excluded_skills` is updated between two matching passes — the pass after the update stops recording new matches for that skill (no subscription/saved-search mutation required).
- [x] 4.4 Add a test for: a subscriber with no profile row (absent from the batch map) matches normally — avoid-skills absence must not break or skip otherwise-valid matches.
- [x] 4.5 Run `go test ./internal/notify/... ./internal/userprofile/...` and confirm all new and existing tests pass.

## 5. Docs

- [x] 5.1 Update `docs/agents/notifications.md`'s "Always true" bullet list with a short note that `notify` matching also gates on the subscriber's live `excluded_skills`, evaluated per-subscriber without an extra search call — so a future reader doesn't reintroduce a per-subscriber Meili filter that would break the O(distinct queries) property.

## 6. Verification

- [x] 6.1 `gofmt -l .` prints nothing for changed files.
- [x] 6.2 `go vet ./...` and `go test ./...` pass.
- [x] 6.3 `go vet -tags=integration ./...` passes.
- [x] 6.4 Verify the new SQL against a real Postgres rather than manually running `go run ./cmd/notify` against the shared local dev stack (this repo checkout's Docker Compose stack is shared with other concurrent sessions — see the "shared workdir hazard" note; avoid disturbing it for an ad hoc run). Added `internal/db/user_profiles_integration_test.go` (`TestListUserProfilesExcludedSkills`), run via `go test -tags=integration ./internal/db/ -run TestListUserProfilesExcludedSkills` against an isolated, throwaway testcontainers Postgres (the project's standard integration-test harness): confirms the `= ANY($1::bigint[])` batch fetch returns each seeded user's `excluded_skills`, an empty list for a user with an empty exclude set, and no row at all for a user with no profile row. The matching logic itself (which (hit, subscription) pairs get excluded) is already covered end-to-end by the `internal/notify` unit tests in section 4, which exercise the real `matchQuery`/`hasAvoidedSkill` code path against fakes.
