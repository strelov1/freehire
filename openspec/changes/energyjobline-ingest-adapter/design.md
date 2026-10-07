## Context

Full recon and the Rigzone feasibility spike live in
`docs/superpowers/specs/2026-10-07-oilgas-job-portal-adapters-design.md`; this design
covers only the part that ships: the `energyjobline` adapter.

`https://www.energyjobline.com/sitemap.xml` is an open sitemap **index** of 4
sub-sitemaps (confirmed via plain `curl` with a Googlebot UA — no bot defense observed). A
live posting, `https://www.energyjobline.com/job/controls-engineer-atlanta-31835232`,
server-renders three `<script type="application/ld+json">` blocks (`WebSite`,
`Organization`, `JobPosting`) carrying `title`, `description`, `jobLocation`/`address`,
`hiringOrganization`, `datePosted`, `employmentType`, `baseSalary`.

The codebase already has two adapters shaped like this exactly:

- `internal/ingest/sources/dataart.go` — sitemap enumeration → bounded-fanout detail
  fetch → `ldJobPosting` decode, for a **single-company boardless** source.
- `internal/ingest/sources/bayt.go` / `gulftalent.go` — a **boardless aggregator**:
  `detail` reads the employer from the posting's own `hiringOrganization.name`
  (`strings.TrimSpace(p.HiringOrg.Name)`), falling back to `unreadableDetail(id, link,
  e.Company)` when that name is empty.

EnergyJobline is the first case where both shapes apply at once: sitemap-enumerated (like
DataArt) *and* a multi-employer aggregator (like Bayt/GulfTalent).

## Goals / Non-Goals

**Goals:**
- Crawl every EnergyJobline posting via its sitemap index into the catalogue, filed under
  each posting's real employer.
- Reuse existing helpers/markers end to end (`sitemapJobLocs`/`resolveSubSitemap`,
  `fetchDetails`, `defaultDetailWorkers`, `ldJobPosting`, `unreadableDetail`,
  `boardless`/`aggregator`) — no new shared mechanism.

**Non-Goals:**
- Rigzone/Oilcareers, NaukriGulf, OilAndGasJobSearch, Laimoon — excluded per the linked
  design doc; not touched by this change.
- Any change to `CompanyEntry`, `Hub`/`Tenants`, or the `aggregator`/`boardless` interfaces
  themselves.
- A shared "sitemap-index aggregator" base type. Two adapters (Bayt/GulfTalent are
  listing-paginated, not sitemap-enumerated) do not yet share enough shape to justify
  extracting one; revisit if a third sitemap-enumerated aggregator shows up.

## Decisions

**Enumeration: sitemap index, not listing pagination.** Unlike Bayt (paginated
per-country listings), EnergyJobline exposes a sitemap index. Use
`resolveSubSitemap`/`getSitemap`'s existing index-vs-flat-urlset handling
(`internal/ingest/sources/sitemap.go`) to walk the 4 sub-sitemaps and collect job URLs via
a job-URL-shape extractor, the same contract `dataart.go` already uses with
`sitemapJobLocs`.

**Company resolution: `hiringOrganization.name` + `unreadableDetail` fallback, not a new
Hub mechanism.** EnergyJobline's `JobPosting` already carries a clean per-posting employer
name — the same situation Bayt/GulfTalent are in — so this copies their resolution
exactly rather than inventing a `CompanyEntry.Hub`-based path (an earlier draft of the
linked design doc proposed that; it was corrected once `bayt.go` was read directly).

**Completeness marker: `aggregator()` only, not `fullBoardListing()`.** `fullBoardListing`
is Bayt's promise that pagination was walked to a genuinely empty page — a pagination-specific
guarantee. EnergyJobline's sitemap already lists every URL in one fetch per sub-sitemap, so
there is no pagination-exhaustion question to prove; the sitemap fetch itself either
succeeds (SHALL error the board on failure, matching Bayt's "first listing page" rule) or
it does not.

**Detail-fetch worker count: `defaultDetailWorkers` (8), not a throttled custom
constant.** Bayt/GulfTalent reduced theirs because Akamai throttling was *observed live*.
No equivalent throttling signal exists yet for EnergyJobline; inventing a lower constant
without evidence would be speculative. Revisit with a real constant + comment if a crawl
run shows throttling.

## Risks / Trade-offs

- **[Unknown job volume]** → the first manual run's job count/duration is checked before
  the board is added via `cmd/add-board`, the same caution the SuccessFactors hub design
  raised for an uncounted hub.
- **[`hiringOrganization.name` has no curation step — a messy-but-present name still gets
  stored]** → accepted, matching Bayt/GulfTalent's accepted risk for the same reason (it
  is the only per-posting employer signal the site offers); watch the catalogue after
  launch.
- **[Sitemap structure could differ from `dataart.go`'s assumption]** → `getSitemap`
  already decodes both `<urlset>` and `<sitemapindex>` shapes through one type, so this is
  a parsing detail, not a design risk — confirmed directly against the live index before
  implementation.

## Migration Plan

1. Ship the adapter + registry line + tests through normal code review (no schema/data
   migration involved).
2. After merge and a first manual run (`go run ./cmd/ingest energyjobline` or equivalent)
   confirms a sane job count, add the live board via `cmd/add-board --provider=energyjobline
   --board='www.energyjobline.com' --company='EnergyJobline' --apply` on the prod host.
3. Rollback is removing the `registry.go` line and retiring the board
   (`cmd/add-board --retire`) — no other system depends on this source.

## Open Questions

None outstanding — the one real unknown this batch had (Rigzone's bot defense) was
resolved by the spike in the linked design doc (INVALIDATED, out of scope for this
change).
