## Why

The board catalog (which company crawls on which ATS, under what board id) lives in
`sources/*.yml`, edited by PR and linted in CI by `cmd/validate-sources`. Separately,
`link_contributions` tracks a crowdsourced submission's lifecycle (`pending` →
`onboarded`/`rejected`, plus a `review` bucket for an unclassified URL) but a `pending`
row is inert — turning it into a live, crawled board still means generating a PR against
the YAML file. Two systems track overlapping state ("is this board known"), and only one
of them (git) actually feeds `cmd/ingest`. Moving the catalog into Postgres collapses
that split: a recognized contribution becomes a catalog row directly, at an unproven
`pending` status, crawled immediately and promoted to `active` on its first success —
no PR, no separate onboarding step.

## What Changes

- New `boards` table replaces `sources/*.yml` as what `cmd/ingest` reads: `provider,
  board, region, company, hub, tenants`, plus a lifecycle `status`
  (`pending`/`active`/`rejected`/`retired`).
- **BREAKING**: `cmd/ingest` no longer accepts a file path / `SOURCES_FILE`. It takes a
  provider name (`go run ./cmd/ingest greenhouse`) and queries `boards` for that
  provider's `pending`/`active` rows. One cron timer per provider, as today.
- A crowdsourced contribution with a recognized `(provider, board)` inserts directly
  into `boards` at `status='pending'`, validated the same way `cmd/validate-sources`
  validates a YAML entry today (provider known, required fields present, not a
  duplicate of an existing non-`retired` row). Failing validation inserts at
  `status='rejected'` with a reason, instead of silently not existing.
- A `pending` board is crawled like an `active` one; its first crawl that completes
  without a board-level error flips it to `active`.
- New `board_submissions` table replaces the `review` slice of `link_contributions` — a
  raw URL nobody has classified into `(provider, board)` yet. Triage resolves the
  `(provider, board)` and inserts into `boards`, then deletes the submission row.
- New `cmd/add-board` worker (report-by-default, `--apply` to write, same convention as
  `cmd/merge-companies`) is how a curator adds or retires a board by hand, replacing
  hand-editing YAML. Retiring a board sets `status='retired'` rather than deleting the
  row.
- One-off `cmd/backfill-board-catalog` worker seeds `boards` from the current
  `sources/*.yml` at `status='active'` before the deploy that removes YAML loading.
- **BREAKING**: `sources/*.yml`, `cmd/validate-sources`, the "Validate sources" CI step,
  and the `onboard-contributions` skill are removed. `link_contributions` (table) is
  dropped once `boards`/`board_submissions` are live.
- Out of scope: reordering ingest work by staleness, per-board (rather than
  per-provider) scheduling, or any cross-provider work-stealing. Scheduling granularity
  is unchanged — this is a storage migration, not a scheduler redesign — and is called
  out as a distinct follow-up in the design doc this proposal is based on.

## Capabilities

### New Capabilities
- `board-catalog`: the `boards` table, its `pending`/`active`/`rejected`/`retired`
  lifecycle, insert-time validation, and the `cmd/add-board` curator worker.

### Modified Capabilities
- `source-ingest`: "Boards to crawl are configured in a file" becomes "configured in the
  `boards` table"; `cmd/ingest`'s invocation contract changes from a file path /
  `SOURCES_FILE` to a provider-name argument.
- `link-contributions`: a recognized contribution now inserts directly into `boards` at
  `status='pending'` (crawled immediately) instead of an inert `link_contributions` row
  awaiting manual onboarding; the unclassified-URL `review` case moves to the
  `board_submissions` table.
- `ingest-board-health`: the requirement stating the catalog lives in YAML is updated to
  say it lives in the `boards` table; `board_health` itself (schema, queries, the
  `(provider, board, region)` key) is unchanged.

## Impact

- **Code removed**: `sources/*.yml` (all files), `cmd/validate-sources`,
  `internal/ingest/sources/config.go`'s YAML parsing (`LoadConfig`, `ParseConfig`,
  `ParseRawEntries`, `dedupeBoards`), the "Validate sources" CI step, the
  `onboard-contributions` skill.
- **Code added**: `boards`/`board_submissions` migrations, new package
  `internal/ingest/boardcatalog` (validation, the `boards` repository, and the DB-backed
  loader `cmd/ingest` uses in place of the YAML loader — not `internal/ingest/sources`
  itself, which `boardcatalog` imports and so cannot import back), `cmd/add-board`,
  `cmd/backfill-board-catalog`, the `pending → active` transition (piggybacked on
  `cmd/ingest`'s existing board-health success write, not a `pipeline.go` change).
- **Code changed**: `cmd/ingest` (provider-name argument instead of file path),
  `internal/ingest/contribution` (`Record`/`RecordReview`/`ListByUser` rewritten against
  `boards`/`board_submissions`), deployment cron/systemd units (one timer per provider
  name instead of per file). **Not** the same count: every provider-only file is a no-op
  rename, but `sources/custom.yml` bundles ~25 distinct providers into one cron timer
  today — splitting it into one invocation per provider means ~25 timers, not one (see
  design.md, corrected during implementation).
- **Data migration**: `cmd/backfill-board-catalog` runs once in prod before the
  YAML-removal deploy; `link_contributions` is dropped in a later migration once
  `internal/ingest/contribution` no longer references it.
- **Full design**: `docs/superpowers/specs/2026-09-03-board-catalog-in-db-design.md`.
