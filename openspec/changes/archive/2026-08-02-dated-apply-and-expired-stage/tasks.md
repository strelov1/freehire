## 1. The `expired` stage

- [x] 1.1 Add `expired` to the vocabulary in `internal/userjob`: `Stages`, `terminalStages`
  (no `activeRank` entry, no `silenceThresholds` entry), the `closed` group and the label
  `Expired`. The three binding tests (`TestEveryStageIsRankedOrTerminal`,
  `TestSilenceThresholdsCoverExactlyTheActiveStages`, `TestEveryStageBelongsToExactlyOneGroup`)
  must pass without being relaxed.
- [x] 1.2 Add a test asserting `Forward` refuses to advance an application out of `expired`,
  so mail classified later cannot resurrect it.
- [x] 1.3 Regenerate the frontend contracts (`make` target or `go run ./cmd/gen-contracts`) and
  confirm `pnpm run check` passes in `web/` with no hand edits to the SPA.
- [x] 1.4 Add a test that `PATCH /jobs/:slug/track` accepts `expired`, and one that an expired
  application reports no silence state. Not an integration test: the stage has no database
  constraint, so Postgres would accept any string — the vocabulary is enforced in the service
  before the write, which is where the test has to sit to prove anything.

## 2. Believable dates, defined once

- [x] 2.1 Move the apply-date bounds (not in the future, not more than a year ago) into
  `internal/userjob` as an exported check, and have `internal/ghostreport` call it instead of
  its private copy. Its existing tests must pass unchanged.

## 3. Re-dating an application

- [x] 3.1 Add the `RedateApplication` statement to `internal/db/queries/user_jobs.sql`: set
  `applications.applied_at` and the `occurred_at` of that application's `applied` event to the
  same instant, touching neither `applied_count` nor `recorded_at`. Regenerate with `make sqlc`.
- [x] 3.2 Add `MarkAppliedOn` to `internal/jobtracking`: in one transaction, mark applied with
  the stated instant, then re-date. Creating and correcting both end with the column and the
  ledger reporting the same instant.
- [x] 3.3 Add an integration test for the correction path. The create-with-a-date half needed
  none: `MarkJobApplied` already carries it for the mail path, and
  `TestMarkJobApplied_DatesTheEventFromTheMessage` covers it. Only the correction was new.
  Original wording: an integration test for the correction path: an application recorded today,
  corrected to last month, has exactly one `applied` event, carrying the corrected date, and an
  unchanged `applied_count`.

## 4. The apply endpoint

- [x] 4.1 Parse an optional `{"applied_on": "YYYY-MM-DD"}` body in `MarkApplied`
  (`internal/handler/user_jobs.go`), convert the day to noon UTC, and route to `MarkAppliedOn`.
  No body, or no date in the body, keeps today's behaviour.
- [x] 4.2 Add handler tests: a stated day is stored at noon UTC; a malformed date, a future
  date and one older than a year are each `400` with nothing recorded.

## 5. CLI

- [x] 5.1 In `../freehire-cli`, give `client.Apply` the optional date and send it as the body.
- [x] 5.2 Add `--on YYYY-MM-DD` to `freehire apply`, with a test covering the parse and the
  request body it produces.

## 6. Documentation and verification

- [x] 6.1 Update `docs/API.md`: the apply body and its bounds, and the widened stage list on
  `PATCH /track`.
- [x] 6.2 Run `go build ./...`, `go vet ./...`, `go test ./...` and `go vet -tags=integration
  ./...`; run the tagged suites for `internal/handler` and `internal/db`.
