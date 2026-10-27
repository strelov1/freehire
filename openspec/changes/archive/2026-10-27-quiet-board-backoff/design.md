## Context

The full approved design, with the interactions checked against the code, is
`docs/superpowers/specs/2026-10-25-quiet-board-backoff-design.md`. This file records the
implementation decisions.

`board_health` holds one row per (provider, lower(board), region). `RecordBoardSuccess` upserts it
on every successful crawl, and `RecordBoardFailure` + `SetBoardCooldown` apply the failure backoff,
whose policy (`pipeline.CooldownFor`) lives in Go. `Runner.cooledDown` reads `GetBoardCooldown`
before each board and fails open. The recovery probe calls `ClearProviderCooldowns`, which nulls
`cooldown_until` for every cooled board of a provider.

## Goals / Non-Goals

**Goals:** back off a board whose listing set is unchanged (4h doubling to 72h), reset on any
appearance or disappearance, keep it independent of the failure cooldown, and report skips.

**Non-Goals:** detecting content edits; per-provider tuning; any scheduler (`ingestsched`) change;
turning on `close-chronic-boards --apply-empty-feed`.

## Decisions

- **Fingerprint = sha256 over the sorted, de-duplicated external ids, hex.** It catches both an
  appearance and a disappearance in one stored value, at no extra query. Rejected: comparing counts
  (misses a swap) and diffing against the stored open rows (one heavy query per board per crawl).
- **`RecordBoardSuccess` becomes `:one` and returns the new `quiet_streak`.** It takes a nullable
  `fingerprint`: NULL means "this crawl cannot vouch for the whole listing" and leaves all three quiet
  columns untouched. The comparison is in SQL, because the stored value is there. The pause length
  is in Go (`pipeline.QuietFor`), mirroring `RecordBoardFailure` → `CooldownFor` → `SetBoardCooldown`.
- **`SetBoardQuietUntil` is CAS-guarded on `quiet_streak`,** for the same concurrent-worker race
  `SetBoardCooldown` documents: a stale writer must not overwrite a newer streak's pause.
- **One read for both pauses.** `GetBoardCooldown` selects `cooldown_until, quiet_until`, and the
  `BoardHealth.Cooldown` port returns both. A second query per board would double the per-board round
  trips over the crawl host's WireGuard link to the database.
- **Separate `quiet_until` column,** so `ClearProviderCooldowns` (which touches only
  `cooldown_until`) cannot erase quiet pauses.
- **Neutral (NULL) fingerprint whenever the crawl is not complete:** `Stats.Failed > 0`,
  `Stats.Interrupted > 0`, the context ended, or the provider is `selfClosing`.

- **Skipped boards are excluded from the company- and source-scoped sweeps** (found while
  implementing). Those closes act on any provider row of a crawled company (or of the whole source)
  unseen for 48h, so a board quiet for up to 72h would have its live postings closed whenever the
  run crawled another board of the same company. The run already knows every board it skipped, so it
  hands their `BoardPattern`s to the sweep, which excludes them with `NOT LIKE ANY`. This also closes
  the same, rarer hole that failure-cooled boards had all along.

## Risks / Trade-offs

- [A dropped posting (a failed detail fetch) changes the fingerprint] → it only resets the backoff,
  which is the safe direction.
- [New postings on a quiet board surface up to 72h late] → this is the accepted cost. The cap was
  chosen by the owner, and any change resets the board to its normal cadence immediately.
- [A recency-budgeted adapter never backs off] → its listing really does change every run, so this is correct.

## Migration Plan

One migration adding `listing_fingerprint text`, `quiet_streak int NOT NULL DEFAULT 0`, and
`quiet_until timestamptz`. All are nullable or have a constant default, so it is a catalogue-only
`ALTER` with no rewrite. `release.sh` migrates before the flip. Rollback: the old binary ignores the
columns. To disable without a deploy, `UPDATE board_health SET quiet_until = NULL` clears every pause,
though they come back on the next quiet crawl.
