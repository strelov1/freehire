## Why

`backend-br/vagas` is a high-traffic Brazilian GitHub-issues job board (~50 open
vacancies at any time). Unpacking its open issues shows the "apply" links resolve to a
handful of Brazilian ATS platforms — most of which we do **not** ingest today, so those
vacancies are invisible to freehire. Of the platforms referenced, only InHire (DB1) is
already covered. The rest are keyless, multi-tenant Brazilian ATS platforms that clear
the same bar as our existing adapters (structured public data, one company per board),
so each is worth a dedicated `Source` adapter. Adding them both closes the coverage gap
for this repo's companies and unlocks the broader BR customer roster each platform hosts.

## What Changes

- Add board-based `Source` adapters for the Brazilian ATS platforms surfaced by
  `backend-br/vagas`, each keyless and multi-tenant, each registered in `sources.All`
  and seeded with the live-validated companies from the repo:
  - **quickin** (`api.quickin.io`) — account-slug board; list endpoint carries the full
    posting inline (no per-job detail request).
  - **mindsight** (`oportunidades.mindsight.com.br`) — Next.js `__NEXT_DATA__` career
    pages; listing is structured, description comes from each posting's detail page.
  - **enlizt** (`<tenant>.enlizt.me`) — schema.org `JobPosting` JSON-LD.
- Each adapter is board-based and multi-tenant (not boardless), so it lists in the source
  facet with no extra wiring — like greenhouse/personio/inhire.
- Regenerate `web/src/lib/generated/contracts.ts` so the new provider keys appear in
  `SOURCE_VALUES` (the frontend source filter is generated from `FilterableProviders()`).

## Capabilities

### New Capabilities
- `backend-br-ats-adapters`: crawl the keyless Brazilian ATS platforms referenced by
  `backend-br/vagas` (quickin, mindsight, enlizt) — one company per board — and normalize
  their open vacancies into the job catalogue, so those postings become searchable
  alongside the rest.

### Modified Capabilities
<!-- None: additive adapters behind the existing Source interface and registry; no existing spec's requirements change. -->

## Impact

- **New code:** one `internal/sources/<provider>.go` (+ `_test.go`) per platform, one
  registration line each in `internal/sources/source.go`, one `sources/<provider>.yml`
  board file each.
- **No schema/API changes:** every adapter emits the same `sources.Job` shape; ingest,
  dedup, enrichment, and search are unchanged.
- **Generated contracts:** `web/src/lib/generated/contracts.ts` regenerated to include
  the new provider keys in `SOURCE_VALUES` (via `make gen-contracts`).
- **Ops:** each new board file is a new `cmd/ingest sources/<provider>.yml` cron entry,
  added the same way as every other provider file.
- **Reuses existing transport/helpers:** `JSONGetter` / `TextGetter` / `HTMLGetter`,
  `sanitizeHTML`, `bracketSlice`, `ldJobPosting`, `fetchDetails`, `workplaceTypeMode`.
- **Descoped (not keyless):** `workfully` (`hiring.workfully.com`, API 403s without a
  session token; RSC-rendered) and `strider` (`app.onstrider.com`, a Clerk-authenticated
  SPA whose API 401s) both sit behind authentication, failing the read-only-public-API bar
  every other adapter clears. Each covers only one company from the repo (Stefanini; one
  US-marketplace recruiter). Recorded as a seam rather than built with a fragile
  anonymous-token workaround.
- **Known seam (out of scope):** bulk harvest of each platform's full BR customer roster
  (this change seeds only the live-validated companies from `backend-br/vagas`), and the
  ~20 `sylision.com` aggregator postings + LinkedIn/one-off links in the repo that map to
  no ATS board (a free-text extraction source, not an adapter, would be needed).
