## Why

Microsoft's careers catalogue at `apply.careers.microsoft.com` is not the old
`careers.microsoft.com` site an earlier feasibility pass marked "blocked" — it is a fresh
front built on **Eightfold AI**, a multi-tenant ATS many companies use. A probe shows two
public, auth-free GET-JSON endpoints behind it:

- **List** — `GET /api/pcsx/search?domain=<domain>&query=&start=<n>&num=10` returns
  `data.positions[]` plus `data.count`. (`num` is server-capped at 10, so `start` is the only
  pagination lever; the higher-level `/api/apply/v2/jobs` list returns `403 PCSX`.)
- **Detail** — `GET /api/apply/v2/jobs/<id>?domain=<domain>` returns the full posting,
  including the `job_description` HTML and a `canonicalPositionUrl`, which the list omits.

Because the endpoints are generic to Eightfold (not Microsoft-specific), the right shape is a
reusable `eightfold` adapter keyed by board — exactly like the host-keyed `jibe`/`icims`
adapters and the host/site-keyed `oracle` adapter — with Microsoft as its first configured
board.

## What Changes

- Add an `eightfold` source adapter (`internal/sources/eightfold.go`) speaking the existing
  `Source` interface, registered with one `NewEightfold(c)` line in `sources.All`.
- It is a **board-based** adapter (NOT boardless). Eightfold needs two values — the public
  host (for request paths) and the required `domain` query parameter (the tenant key) — so the
  board id is `"host/domain"`, e.g. `"apply.careers.microsoft.com/microsoft.com"`, parsed the
  same way `oracle` splits `"host/site"`.
- Eightfold has **two list-API generations** and a tenant supports exactly one (the other
  returns `403`): the newer `GET /api/pcsx/search` (positions under `data`, `postedTs` date —
  e.g. Microsoft, Micron) and the legacy `GET /api/apply/v2/jobs` (top-level positions,
  `t_create` date — e.g. Netflix). The adapter **auto-detects**: it tries the pcsx list and, on
  error, falls back to the v2 list, so a board is just `host/domain` for either generation. The
  detail endpoint `/api/apply/v2/jobs/<id>` is shared by both.
- List paging is by `start` (page size fixed at 10 by the server) until a page yields no
  positions or the running count reaches the catalogue total. The list omits the description,
  so each position's detail is fetched (bounded concurrent fan-out via the shared
  `fetchDetails`) for the `job_description` HTML.
- Each posting maps to the normalized job shape: `external_id` = Eightfold's numeric position
  id; `url` = the detail's `canonicalPositionUrl` (falling back to
  `https://<host>/careers/job/<id>`); `title`, `location` (first of the list `locations`), and
  `posted_at` (from the list `postedTs` Unix-epoch) from the list; `work_mode` from the list
  `workLocationOption` via the existing `workplaceTypeMode` helper; `description` = sanitized
  HTML from the detail's `job_description`.
- Add a `sources/eightfold.yml` board file (Microsoft, Micron on pcsx; Netflix on the legacy
  list).
- Regenerate the web TS contracts (`make gen-contracts`): `eightfold` is a non-boardless
  provider, so `sources.FilterableProviders()` adds it to `SOURCE_VALUES` automatically.

## Capabilities

### New Capabilities
<!-- None. Reuses the source-ingest pipeline and write path unchanged. -->

### Modified Capabilities
- `source-ingest`: add a requirement that `eightfold` is a registered board-based provider — a
  list+detail adapter over the Eightfold `/api/pcsx/search` and `/api/apply/v2/jobs/<id>`
  endpoints, keyed by a `"host/domain"` board, yielding the normalized job shape with a
  sanitized-HTML description and paging until the catalogue is exhausted.

## Impact

- **New code**: `internal/sources/eightfold.go` + `internal/sources/eightfold_test.go`; one
  registration line in `internal/sources/source.go` (`sources.All`).
- **Config**: a new `sources/eightfold.yml` board file (Microsoft, Micron, Netflix). No new env
  vars.
- **DB**: none — reuses `UpsertJob` (`source = "eightfold"`, namespaced `external_id`). No
  migration.
- **Contracts**: `web/src/lib/generated/contracts.ts` regenerated (adds `eightfold` to
  `SOURCE_VALUES`).
- **Dependencies**: none — uses the existing shared `HTTPClient.GetJSON`, `fetchDetails`,
  `workplaceTypeMode`, `parseEpochSeconds`, and `sanitizeHTML`.
- **Deploy**: the adapter compiles into the existing ingest binary, so a new adapter needs an
  image rebuild + redeploy (not just a sources rsync) to run in prod, plus a cron schedule for
  the new board file.
- **Out of scope (known seams)**: the `/api/pcsx/search` list omits the description, so the
  crawl costs one detail request per posting (≈1 list page + N details), the same cost profile
  as `oracle`/`icims`. No work-site filter param is used — the crawl walks the whole catalogue
  (empty query). Other Eightfold tenants are added later as more `sources/eightfold.yml`
  entries, no code change.
