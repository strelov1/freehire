## Context

selfrecruit.ge has no JSON API (confirmed live against `dressup.selfrecruit.ge`): the
listing and every detail page are server-rendered HTML with no schema.org/ld+json
markup. The listing is paginated by OFFSET, not page number:
`https://<board>.selfrecruit.ge/vacancies/<offset>` in steps of 10
(`/vacancies/0` ≡ the tenant root `/`), confirmed live across three pages (offsets 0, 10,
20 — dressup has 22 open postings) with a page past the end (`/vacancies/30`) answering
302 back to `/` rather than an empty page. `internal/ingest/sources/html.go` already
carries the generic DOM-walking helpers (`walk`, `attr`, `textContent`, `firstByClass`,
`innerHTML`) shared by `successfactors`, which is the closest existing shape to what a
detail fetch needs here — DOM extraction by class, not a structured payload.

## Goals / Non-Goals

**Goals:**
- Crawl a selfrecruit.ge tenant's open postings with a title, HTML description, and
  stable dedup id, paging the listing to exhaustion.

**Non-Goals:**
- No tenant-discovery/harvest prober. Only one live tenant (`dressup`) is known today;
  most of this codebase's `harvest` probers exist for platforms already carrying dozens
  of tenants (traffit, keka), which doesn't apply here yet. Add one later if more
  tenants surface.

## Decisions

- **DOM extraction, not a `_ats_template`-style generic scraper.** Each detail page's
  title/description sit in fixed, classed elements (`vacancy_title_inner`,
  `pub_vac_text_detail` — confirmed identical across three sampled postings) with no
  `itemprop` or ld+json to decode instead. Following the `successfactors` pattern of a
  bespoke `Fetch` over the shared `html.go` walkers is more maintainable than inventing a
  generic CSS-selector config this codebase doesn't otherwise use.
- **Pagination via `crawlAllPagedLinks`, not the partial-result `crawlPagedLinks`.** A
  paginated listing earns the `fullBoardListing` marker only if a mid-listing failure
  aborts the whole `Fetch` rather than returning a truncated success — the same reasoning
  `hrmos-source` applied to its own `?page=N` walk. The redirect-past-the-end behavior
  (`/vacancies/30` → 302 → `/`) needs no special casing: the walk's own "a page added no
  NEW link" stop condition already ends the loop there, since every link on the
  redirect's landing content was already seen on page 1.
- **Page URL is always `/vacancies/<offset>`, including offset 0** — confirmed byte-for-
  byte identical to the bare tenant root — so the adapter needs no page-1-is-different
  special case.
- **`ExternalID` = the detail URL's UUID path segment** (`/<uuid>` at the tenant root,
  confirmed from the original submission's own URL shape — not the `/articles/<uuid>`
  links also present on the site, which turned out to be unrelated CMS content pages),
  mirroring how every other detail-URL-keyed adapter (e.g. `careerspage`'s `/jobs/<uuid>`)
  derives a stable id.

## Risks / Trade-offs

- [Only one tenant confirmed] → not enough surface to prove the DOM class names hold on a
  differently-themed tenant (selfrecruit.ge's CSS variables suggest per-tenant branding,
  but the underlying template could still vary). Mitigation: the adapter degrades to an
  empty title/location/description rather than erroring when a fixed-class extraction
  comes back empty, matching this codebase's general "partial data over a dropped
  posting" posture; a tenant whose extraction comes back consistently empty is visible as
  a suspiciously thin board rather than a crawl failure.
- [Offset pagination has no explicit "last page" signal other than a redirect] → if a
  future tenant's redirect-past-the-end lands somewhere OTHER than a page whose links are
  already fully seen (e.g. a distinct "no more jobs" page with a couple of unrelated
  promotional links), the walk could in principle keep paging until `selfrecruitMaxPages`.
  `crawlAllPagedLinks` turns hitting that cap while still finding new links into an ERROR
  (failing the whole `Fetch`), not a silent truncation — the correct, safe behavior for a
  `fullBoardListing` adapter, but it means such a tenant's board would fail outright on
  every crawl rather than merely under-report, until the cap or the matcher is revisited.

## Migration Plan

No data migration. Ship the adapter, merge, deploy, then add the `selfrecruit/dressup`
board by hand via `cmd/add-board` (per the existing curator workflow) to close
`board_submissions` id 8. Rollback is deleting the source registration; no board exists
yet for the adapter to leave orphaned.
