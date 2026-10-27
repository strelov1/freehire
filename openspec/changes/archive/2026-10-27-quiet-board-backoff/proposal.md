## Why

Every crawl of a provider re-reads every one of its boards, whether or not the board ever changes. On
2026-10-25 `close-chronic-boards` reported 7,229 boards whose crawls all succeed and yield nothing, and
they are re-read every cycle by a crawl fleet whose capacity is the host's bottleneck. The only per-board
pacing today is the failure backoff, so a board that answers correctly but has nothing new is never
paced at all.

## What Changes

- A board whose listing (the set of postings it names) has not changed since its previous complete
  successful crawl is crawled progressively less often: 4h, 8h, 16h, 32h, 64h, capped at 72h.
- The first complete crawl that sees a posting appear or disappear resets the board to the
  provider's normal cadence.
- The quiet pause is stored apart from the failure cooldown, so the provider-wide recovery probe, which
  clears cooldowns, never erases it.
- An incomplete crawl (interrupted, failed, context ended) neither grows nor resets the quiet state.
- Self-closing providers (which emit only changed postings) are never paced this way.
- Every run reports how many boards it skipped as quiet.

## Capabilities

### New Capabilities

### Modified Capabilities
- `ingest-board-health`: adds a quiet-board backoff driven by a listing fingerprint, alongside the
  existing failure cooldown, and adds skipping a board in its quiet pause.

## Impact

- `migrations/`: a new migration adding `listing_fingerprint`, `quiet_streak`, `quiet_until` to
  `board_health` (defaulted/nullable, no table rewrite).
- `internal/platform/db/queries/board_health.sql`: `RecordSuccess` takes and compares a
  fingerprint and returns the streak; new statement to set `quiet_until`; `Cooldown` reads both pauses.
- `internal/ingest/pipeline`: fingerprint computation, `QuietFor` policy, the skip, `Stats.Quiet`.
- `cmd/ingest/board_health.go`: the `BoardHealth` adapter.
- No API, UI or search changes. The `ingestsched` scheduler is untouched.
