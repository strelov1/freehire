## Why

`cmd/harvest-boards` already expands a provider's board catalog from a seed slug list, but
every seed today is hand-written. Public, MIT-licensed ATS-company inventories exist
(e.g. kalil0321/ats-scrapers' `ats-companies/<provider>.csv` files — `name,slug,url` rows,
one file per ATS platform) that could seed dozens of providers freehire already crawls, but
there is no tool that turns such a CSV into the JSON shape `harvest-boards` consumes.
Hand-converting thousands of rows per provider is not viable, and guessing each platform's
board-id format from the URL by hand would duplicate logic `internal/ingest/atsboard`
already owns and tests.

## What Changes

- Add `cmd/seed-from-inventory`, a new one-off Go tool that reads a `name,slug,url` CSV and
  writes one `harvest-boards`-shaped seed JSON file per recognized provider.
- For each row, resolve the freehire provider and board id via the existing
  `internal/ingest/atsboard.Recognize(url)` — no new URL-parsing logic, no per-provider
  special-casing beyond what `atsboard` already encodes.
- Rows `atsboard.Recognize` cannot resolve (vanity domains, custom-domain ATS such as Taleo,
  SuccessFactors, or Oracle on their own domain — a documented limitation of `atsboard`) are
  skipped and counted in a summary, never fatal to the run.
- The tool touches no database and makes no network call, mirroring `atsboard`'s own
  "no network, no database" contract — it is a pure local file transform.

## Capabilities

### New Capabilities
- `board-inventory-seed`: converting an external `name,slug,url` ATS-company inventory CSV
  into per-provider `harvest-boards` seed JSON files via `atsboard.Recognize`.

### Modified Capabilities
(none — `harvest-boards`' own seed-consumption and validation behavior is unchanged)

## Impact

- New package/binary: `cmd/seed-from-inventory` (plain `main`, no `worker.Main` — needs
  neither `DATABASE_URL` nor network access).
- Depends on the existing `internal/ingest/atsboard` package (read-only use of its exported
  `Recognize` function; no changes to that package).
- Output feeds the existing, unmodified `cmd/harvest-boards <provider> <seed.json>` flow.
- No schema, migration, API, or deployment changes. No CI/workflow changes anticipated
  beyond the standard `go build`/`go vet`/`go test` coverage of a new `cmd/` package.
