## Why

Most ATS adapters set `Job.Company = e.Company` straight from the board file, and a large share of `sources/*.yml` entries carry a squished slug in that field instead of a real name (e.g. `lbresearch`, `gs1ca`, `afcb`). That slug then leaks everywhere the company is shown: the UI label, the public `/companies/<slug>` URL, OG cards, and — because the SPA resolves logos through logo.dev's *name* endpoint — it fails to resolve (404) so the logo silently degrades to a monogram. Pinpoint was fixed by a one-off manual batch (PR #825, 105 names harvested from careers-page titles); the same defect spans tens of thousands of entries across bamboohr (~7223), workday (~5029), ashby (~2712), lever (~1624) and others. A hand-edit-per-file approach does not scale and rots as new slug boards arrive.

## What Changes

- Add a run-once-and-exit worker `cmd/backfill-company-names` that finds companies whose stored name is still slug-like and replaces it with the company's real display name resolved from that ATS's own native source.
- Per-ATS resolvers: careers-page HTML `<title>` (BambooHR/Lever/Ashby/Pinpoint, format like `Jobs at {Name} | {Name} Careers`) and, where the ATS exposes it, an API field (Greenhouse board `name`, Workday). iCIMS already prefers `HiringOrganization.Name` and needs no backfill.
- A shared name-resolution gate: decode HTML entities, reject garbage (recruiter names, test boards, empty titles), and require a confidence match (returned name shares tokens or an acronym with the slug) before accepting a name.
- Only rewrite companies whose current name is still slug-like AND that have live jobs — dead/empty boards are skipped so effort tracks real UX impact.
- Dict-only spirit: never invent or prettify a slug into a name; when no trustworthy source name is found, leave the company untouched.

## Capabilities

### New Capabilities
- `company-name-backfill`: A maintenance worker that resolves and applies real company display names for boards whose ingested company name is a slug, sourced deterministically from each ATS's native title/API with a confidence gate.

### Modified Capabilities
<!-- none: display, companies table, and logo resolution behavior are unchanged; this only improves the stored name value -->

## Impact

- New: `cmd/backfill-company-names/main.go`; a resolver package under `internal/` (e.g. `internal/companyname/`) with per-ATS strategies and the confidence/entity-decode gate.
- Reuses existing DB layer: reads slug-like companies with live jobs, updates `companies.name` / `jobs.company`; the derived catalogue reconciles through the existing `SyncCompaniesFromJobs` + `DeleteOrphanCompanies` queries.
- Reuses the existing sources HTTP client (proxy/User-Agent) for careers-page and API fetches.
- Operational: a new cron-style run-once worker (needs `DATABASE_URL`, optionally the sources proxy); follow with `make reindex` so corrected names reach Meilisearch. No schema migration required.
- No API-contract or frontend changes.
