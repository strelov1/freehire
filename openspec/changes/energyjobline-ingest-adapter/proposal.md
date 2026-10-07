## Why

Oil & gas is a vertical the business is actively growing (ChatGPT traffic, SEO on roles).
A recent audit of ~40 oil & gas domains found most majors already covered or added, and
most remaining job portals either already live (Bayt, GulfTalent), dead (Laimoon), or
unreachable at the network level (NaukriGulf, OilAndGasJobSearch, and — per a live
feasibility spike — Rigzone). EnergyJobline is the one portal left that is both reachable
with plain HTTP and offers clean per-posting structured data, so it is the one worth
building now. Full findings: `docs/superpowers/specs/2026-10-07-oilgas-job-portal-adapters-design.md`.

## What Changes

- Add a new `energyjobline` ingest source adapter
  (`internal/ingest/sources/energyjobline.go`) that enumerates
  `https://www.energyjobline.com/sitemap.xml` (a 4-entry sitemap index), fetches each
  job-detail page, and parses its embedded schema.org `JobPosting` JSON-LD.
- The adapter is a **boardless aggregator**: one crawl covers the whole site (no
  per-tenant board id), and each posting's employer comes from the posting's own
  `hiringOrganization.name` — the same pattern `bayt.go`/`gulftalent.go` already use, reusing
  the existing `unreadableDetail` helper as the fallback when that name is empty.
- Register `energyjobline` in `internal/ingest/sources/registry.go`'s `All()`.
- After the code ships and a first manual run looks sane, add the live board via
  `cmd/add-board --provider=energyjobline --board='www.energyjobline.com' --company='EnergyJobline' --apply`
  on the prod host (a manual post-merge step, not part of this change's tests).

## Capabilities

### New Capabilities

- `energyjobline-source`: crawls EnergyJobline's sitemap-enumerated job postings into the
  catalogue, resolving each posting's employer from its own `JobPosting` JSON-LD as a
  boardless, multi-company aggregator source.

### Modified Capabilities

(none — this adds a new adapter and does not change the behavior of any existing source,
the `aggregator`/`boardless` marker interfaces, or `CompanyEntry`)

## Impact

- New file `internal/ingest/sources/energyjobline.go` (+ `energyjobline_test.go`).
- One new line in `internal/ingest/sources/registry.go`'s `All()`.
- One new `boards` row added via `cmd/add-board` post-merge (prod data change, not code).
- No change to any existing adapter, to `CompanyEntry`, or to the `aggregator`/`boardless`
  marker interfaces — this change only consumes them.
- Out of scope: Rigzone/Oilcareers, NaukriGulf, OilAndGasJobSearch, Laimoon — see the
  linked design doc's "Out of scope" section for why each is excluded from this change.
