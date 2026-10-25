## 1. Policy and fingerprint (pure Go)

- [x] 1.1 `pipeline.QuietFor(streak)`: 0 → no pause; 4h·2^(streak−1) capped at 72h; overflow-safe. Unit tests for the sequence, the cap, and streak 0.
- [x] 1.2 `pipeline.listingFingerprint(ids []string) string`: sha256 hex of the sorted, de-duplicated ids; "" for no ids is still a valid fingerprint distinct from "neutral". Tests: order and duplicates do not matter, and one id added or removed changes it.

## 2. Storage

- [x] 2.1 Migration: add `listing_fingerprint text`, `quiet_streak int NOT NULL DEFAULT 0`, `quiet_until timestamptz` to `board_health`. Passes `pnpm check:sql`.
- [x] 2.2 SQL: `GetBoardCooldown` also returns `quiet_until`. `RecordBoardSuccess` takes a nullable fingerprint, applies the three branches and returns `quiet_streak` (`:one`). New `SetBoardQuietUntil`, CAS on `quiet_streak`. Run `make sqlc`.
- [x] 2.3 Integration tests (tagged): the three `RecordBoardSuccess` branches, the CAS, and that `ClearProviderCooldowns` leaves `quiet_until` alone.

## 3. Port and adapter

- [x] 3.1 `pipeline.BoardHealth`: `Cooldown` returns both pauses, and `RecordSuccess` takes the fingerprint (nullable). Update `cmd/ingest/board_health.go` to call `QuietFor` and `SetBoardQuietUntil` from the returned streak, and update every fake and caller (`go vet -tags=integration ./...`).

## 4. Runner

- [x] 4.1 Collect the listed external ids in both paths (buffered `raw`, streaming emit sink). Pass a fingerprint only for a complete crawl (no Failed, no Interrupted, the context not ended, not `selfClosing`), otherwise neutral. Runner tests against the fake BoardHealth.
- [x] 4.2 Skip a board whose quiet pause is in the future (fail open), count it in a new `Stats.Quiet` (merged like `Cooled`), and log "skipped N quiet boards" in the run summary. Runner tests.

- [x] 4.3 Record every skipped board (quiet or cooled) in `Stats.SkippedBoards` as its `externalid.BoardPattern`, merged across boards. Exclude them from `CloseUnseenJobs`, `UnseenJobIDs`, `CloseUnseenJobsBySource` and `UnseenJobIDsBySource` with `NOT (external_id LIKE ANY(...))`, and pass them from `cmd/ingest`'s sweep. Tests: an integration test that the excluded board's stale rows survive both scopes, and a unit test that skipped boards reach the sweep.

- [x] 4.4 Review fixes: no fingerprint when a posting was Unreadable or failed to save, or when a hydrating adapter left a listed NEW posting un-hydrated (budget or detail failure, detected via the seen() predicate). A quiet board reads as fresh in ProviderHealthRollup (/status) and ProviderIngestHealth (alerts).

## 5. Docs and ship

- [x] 5.1 Document the quiet backoff in `internal/ingest/pipeline/AGENTS.md` (beside the cooldown) and in `docs/agents/job-lifecycle.md`'s interactions.
- [ ] 5.2 Full verification: `gofmt`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./...`, the tagged board-health integration tests. PR, CI, merge, deploy, then check on prod that the quiet columns fill and skips are logged.
