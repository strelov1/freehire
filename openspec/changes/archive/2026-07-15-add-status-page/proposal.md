## Why

The ingest fleet's health already lives in the `board_health` table (per-board
last outcome, cooldown, failure count), but it is only ever surfaced in a
per-run summary log — there is no way for anyone to *see* how the parsing is
doing. We want a public `/status` page that answers "are the sources healthy?"
at a glance, rolled up per ATS provider.

## What Changes

- Add a read-only, per-provider rollup query over `board_health` (grouped by
  `provider`: total boards, healthy boards, boards in cooldown, last run,
  last success, ingested total).
- Add a pure status-derivation policy (in Go) that maps each provider's healthy
  fraction and success-freshness to `operational` / `degraded` / `down`, plus an
  overall fleet status.
- Add a public, unauthenticated `GET /api/v1/status` endpoint returning the
  sanitized rollup (no raw error text, no board identifiers).
- Add a public `/status` page in the web app: an overall banner + a flat list of
  providers with a status pill, board counts, and relative last-run time.

## Capabilities

### New Capabilities
- `ingest-status-page`: A public status surface — a per-provider health rollup
  over `board_health`, a status-derivation policy, a public `GET /api/v1/status`
  endpoint, and a `/status` web page.

### Modified Capabilities
<!-- None: board_health behavior (recording, cooldown) is unchanged; this change
     only adds a read surface over the existing snapshot. -->

## Impact

- **DB / queries**: new sqlc query `ProviderHealthRollup` in
  `internal/db/queries/board_health.sql` (read-only; no migration — the table
  already exists).
- **Backend**: new `internal/handler/status.go` (handler + status-derivation
  policy) and a public route wired next to `/health` in `internal/handler/handler.go`.
- **Frontend**: new `web/src/routes/status/` page consuming `/api/v1/status`.
- No new tables, no writes, no auth surface. Existing ingest/cooldown behavior
  is untouched.
