## Why

The "Site status" section on the public `/status` page (`openspec/specs/site-health-status`)
currently only shows a live "right now" verdict. Real status pages (GitHub Status,
Cloudflare Status) also show a history strip — a row of daily tiles a visitor can
scan to see "has this been reliable lately." Without it, a visitor who lands during
a brief, already-resolved blip has no way to tell it was brief, and one who lands
during a genuinely rare outage has no way to tell it's unusual.

## What Changes

- Add a `site_status_daily` table (one row per calendar day, holding the WORST
  status observed that day) and a background sampler inside `cmd/server` — a
  ticker goroutine (same shape as the existing `startSuggestRefresh`), not a new
  cron binary — that ticks every 5 minutes, computes the site's current status the
  same way `GET /api/v1/status` already does, and upserts the day's worst-so-far.
- Extend the existing `site` object in `GET /api/v1/status` with `site.history`: up
  to 90 daily entries, oldest first, `{ day, status }`. A day with no recorded
  sample is simply absent — never backfilled as a fake "operational."
- Extend `StatusBoard.svelte`'s "Site status" section with a row of up to 90 daily
  tiles below the live banner, color-coded the same way the rest of the page
  already is, plus a distinct "no data" treatment for absent days.
- No new cron binary, no `freehire-ops` change, no uptime-percentage figure, no
  history for the ingest fleet (`overall`/`providers` are unaffected).

## Capabilities

### New Capabilities
(none — this is additive to an existing capability, not a new one)

### Modified Capabilities
- `site-health-status`: adds the daily sampling requirement and the `site.history`
  field to the existing site-status derivation and reporting.
- `ingest-status-page`: the `GET /api/v1/status` response shape and the `/status`
  page's "Site status" section both gain the history data — same endpoint and
  page this capability already governs.

## Impact

- New migration `0144_site_status_daily.sql`; new sqlc queries in
  `internal/platform/db/queries/site_status_daily.sql` (regenerated via
  `make sqlc`).
- `internal/api/handler/status.go` (extract the shared site-status computation so
  both the handler and the new ticker call it; add `history` to the `siteHealth`
  DTO).
- `cmd/server/main.go` (the new ticker, following `startSuggestRefresh`'s shape).
- `web/src/lib/types.ts`, `web/src/lib/components/StatusBoard.svelte`.
