## Context

freehire ingests jobs through single-responsibility `Source` adapters registered in
`sources.All`, each a read-only reader over a public feed. A spike confirmed Djinni exposes
its whole guest corpus (~488 pages × 15 postings) as embedded JSON-LD `JobPosting` arrays on
`/jobs/?page=N`, with full descriptions on the listing, stable numeric ids, working
anonymous pagination, and no anti-bot wall.

The user's requirement — "don't re-ingest what an ATS already gives us" — is already a
solved, specced capability in this codebase (`aggregator-ats-dedup` + the fuzzy/subset title
variants), implemented in the **reindex** suppression pass, not in adapters. A crucial
constraint from the spike: Djinni handles applications in-platform, so postings carry **no
outbound ATS apply URL**; the apply-URL → `atsdetect.FromURL` reroute path (used by
link-source adapters) therefore has nothing to bite on and is not applicable here.

## Goals / Non-Goals

**Goals:**
- A `djinni` adapter that maps the listing JSON-LD to normalized `Job`s and pages to the end.
- ATS-dedup that reuses the existing reindex suppression by classifying `djinni` as an
  aggregator — no new dedup code, no adapter DB access.
- Keep the adapter a pure HTTP reader, unit-testable against a saved HTML fixture.

**Non-Goals:**
- No new cross-source dedup mechanism, DB-backed port, or company registry.
- No per-posting detail hydration (the listing already carries the description).
- No sharding by specialization (single board is acceptable at ~16 min/full crawl); the
  seam is noted but not built.
- No `cmd/backfill-*` worker (nothing to backfill for a brand-new source).

## Decisions

### Decision: Inherit ATS-dedup via the `aggregator` marker, not a company-skip callback

An earlier framing proposed skipping every Djinni posting whose company already has a
registered ATS board (a DB-backed callback into the adapter). Investigation of the existing
specs showed the codebase already suppresses aggregator/ATS twins at reindex, keyed on
`company_slug` + normalized title + compatible country, self-healing when the ATS twin
closes. Marking `djinni` with the existing `aggregator()` marker (and thus surfacing it in
`sources.AggregatorProviders()`) enrolls it in that pass for free.

- **Why over the company-skip callback:** the callback suppresses at company granularity and
  would hide Djinni-exclusive roles at any company we also crawl via an ATS — the exact risk
  flagged when that option was considered. The reindex pass suppresses at posting granularity
  (title + country), so exclusives survive. It also keeps the adapter a pure HTTP reader with
  no DB dependency, matching every other adapter and keeping tests offline.
- **Alternative considered — apply-URL reroute (`atsdetect.FromURL`):** rejected because
  Djinni has no outbound ATS apply URL to detect.

### Decision: Parse the listing JSON-LD; accept both array and single object

Each page has one `ld+json` block that is a JSON array of `JobPosting`. The adapter unmarshals
into `[]jobPosting`, falling back to a single object when the payload is not an array (a
defensive shape other JSON-LD sources in this repo also tolerate). Fields map directly:
`identifier`→`ExternalID`, `url`→`URL`, `title`, `description`(sanitized via the shared
`sanitizeHTML`), `hiringOrganization.name`→`Company`,
`applicantLocationRequirements.address.addressCountry`→location, `datePosted`→
`PostedAt`, `employmentType`/`jobLocationType`→work-mode/type hints mapped through existing
helpers where a clean mapping exists (empty otherwise, so dictionaries decide). The
`hiringOrganization.sameAs` (company website) is deliberately dropped: the `Job` aggregate has
no company-website field (company homepage is owned by the separate `company_info` mechanism),
and the ATS-suppression pass keys on `company_slug` derived from the company name, not the site.

### Decision: Stop pagination on the past-the-end redirect, not on an empty page

The obvious stop — "page until a page yields no `JobPosting`" — is WRONG for Djinni and was
caught only by exercising the real end of the feed (the spike verified the data but not the
boundary). A past-the-end `?page=N` returns `302 → /jobs/`, and Go's HTTP client follows the
redirect, re-serving page 1 (which is NOT empty); the naive stop would then loop, re-ingesting
page 1 up to the page cap. The adapter therefore fetches via `GetHTMLResolved` and stops when
the resolved FINAL URL no longer carries the `page=N` marker — a URL-level signal robust to
Djinni re-ordering the feed by freshness mid-crawl (a content/first-id signature would be
fragile against that). This needed a new `HTMLResolvedGetter` interface (added to the
`HTTPClient` composition; the concrete `*Client` already implements `GetHTMLResolved`). A
non-redirected empty page is kept as a secondary stop, and `djinniMaxPages` (above the observed
~488) is the backstop against a pathological feed, mirroring `justJoinMaxPages`.

### Decision: Pace the crawl and keep partial results on a datacenter 403

The spike (residential IP) saw no rate limiting, but the FIRST prod run (host-2 datacenter IP)
`403`'d at page ~204 of a no-delay crawl — Djinni throttles a fast burst by IP. Two changes
make the crawl production-viable: (1) a `djinniPageDelay` (~600ms) between page requests to stay
under the limit and be a polite crawler; (2) **partial-on-error** — a mid-crawl fetch error
stops the crawl but keeps the pages already collected (the freshest, since Djinni orders by
recency), instead of the original "abort the whole board on any page error" which lost ~3,000
already-crawled jobs and dropped the board into cooldown. The board fails hard ONLY when page 1
fails, so an empty successful crawl never reaches the unseen-sweep (the workday-total:0
false-close class). Trade-off: the crawl depth per run is whatever stays under the limit (the
freshest N pages), not guaranteed full corpus; the older tail is lower value and a residential
egress / deeper pacing is the noted seam if full coverage is ever wanted.

## Risks / Trade-offs

- **Full-corpus crawl is ~488 sequential requests.** → Paced at ~600ms/page (~5 min) and
  bounded by partial-on-error when Djinni's datacenter rate-limit lands; each run reliably gets
  the freshest pages. Sharding by `?primary_keyword=` or a residential egress is a noted seam
  if full-corpus coverage ever matters.
- **A moving 403 boundary causes minor unseen-sweep churn** (jobs near the boundary flip in/out
  between runs). → The 48h unseen window absorbs it — a job must be missed for 48h straight to
  close, which a near-boundary posting rarely is.
- **One failing board = the whole source in cooldown** (single-board granularity of
  `board_health`). → Inherent to a single-board source; consistent with other boardless
  aggregators (justjoin, himalayas).
- **JSON-LD shape drift** (Djinni changes its microdata). → The adapter fails the crawl
  loudly (parse error surfaces as a board failure/cooldown) rather than silently ingesting
  garbage; a fixture-based unit test pins the current shape.
- **Country-only location** from `addressCountry`. → Matches the granularity Djinni exposes;
  the geography dictionary derives country/region facets downstream as for every source.

## Migration Plan

- Pure addition. Deploy the binary, then add a `cmd/ingest sources/djinni.yml` cron schedule
  in freehire-ops. No DB migration, no API change.
- **Rollback:** remove the cron schedule and the `sources.All` registration; existing Djinni
  rows age out via the standard unseen sweep. No data migration to undo.

## Open Questions

- None blocking. Sharding by specialization and a possible description-language signal are
  deferred until there is a concrete need.
