# Oil & gas recruitment agency adapters: Orion, Airswift, Brunel, NES Fircroft

**Date:** 2026-10-08
**Status:** design approved

## The problem

The original ~40-domain oil & gas recon (see
`docs/superpowers/specs/2026-10-07-oilgas-job-portal-adapters-design.md`) deferred a
5-agency batch — NES Fircroft, Airswift, Brunel, Petroplan, Orion — because each runs its
own internal vacancy search, not a known ATS. A fresh recon pass (2026-10-08) against all
five, applying the energyjobline lesson (always probe with both a neutral UA and a
Googlebot UA, never trust one request), found:

- **Petroplan is dead.** Its legacy job-board routes (`/jobs`, `/jobs/details/...`,
  category landing pages) all return the same WordPress "critical error" 500, identically
  for both UAs, at both `www.petroplan.com` and bare `petroplan.com`. The company was
  acquired by TXM Group in mid-2024 and the old vacancy engine was never carried over —
  `wp-json/wp/v2/types` confirms no `job` custom-post-type exists on the current site. Out
  of scope; revisit `txmgroup.com` later if this source still matters.
- **The other four have no bot defense at all** — no Cloudflare challenge, no UA-gating,
  no Crawl-delay. None need the energyjobline-style workaround.
- **None of the four exposes a real end-client per posting.** Checked directly:
  Airswift's `hiringOrganization.name` is the literal string `"Airswift"` on every
  posting; Brunel's `hiringOrganization` is a bare `@id` reference to the site's own
  `#organization` node (not even a name field); NES Fircroft's `company.name` is always
  `"NES Group"`. This is normal recruitment-agency anonymization (the end client is
  confidential), not a data-quality bug. All four are **boardless single-company**
  sources — the same shape as the existing `emagine.go`/`NewStaffy`/`NewLumenalta`
  adapters — not aggregators. No `hiringOrganization`-based company resolution is needed
  anywhere in this batch.

## Orion (orionjobs.com)

Best case of the four: an open, unauthenticated JSON API —
`GET https://www.orionjobs.com/api/recruitment/job/data/?folder=uk&hasexpired=false&page=N`
— returns paginated (10/page) JSON with `id, url, slug, reference, title, description,
locationtext, locationsearch, salarytext, employment_type, discipline`, plus a
`pagination.total` count (220 at check time). No UA requirement, no auth header. The
sitemap (`/api/sitemap.xml`, ~230 `/job/` entries) is a weaker, partially-stale
alternative — some of its entries 410; the API's own `hasexpired=false` filter is the
correct source of truth for "currently open," so the adapter uses the API as the sole
list source and does not touch the sitemap at all.

Shape: same as `emagine.go` (`list JSONGetter` — offset/page-based, not `PostJSON`, since
the API is GET-only with a `page=N` query param — paginate until a page returns fewer than
10 or `page*10 >= pagination.total`). No separate detail call needed: the listing payload
already carries the full description, unlike emagine's list-then-hydrate shape.

## Airswift (airswift.com)

`sitemap.xml` (one flat urlset, ~1841 `/jobs/<slug>-<id>` entries) + per-posting
`JobPosting` ld+json — the `dataart.go` shape exactly. The one real wrinkle: roughly
55-60% of sitemap entries are expired postings that still return `HTTP 200` with no
ld+json block and visible "no longer accepting applications" text, rather than a 404.

**This must be a drop, not an `unreadableDetail` stub.** `unreadableDetail` exists for
"could not confirm whether this posting is still open" (a transient fetch error, a
missing/malformed block when the page's own state is ambiguous) — a confirmed-expired
posting is the opposite: the page affirmatively states the posting is gone, the same
category 404/410 already drops in `bayt.go`. Detect it the same way the content already
signals it: `ldJobPosting` returning false here is EXPECTED at this rate (not a parsing
failure to investigate), so `detail` checks the page text for the site's own expiry
marker before falling back to a stub, and drops outright when it matches. (Confirm the
exact marker text against a live expired posting during implementation — recon saw text
to this effect but did not pin the literal string.)

## Brunel (brunel.net)

`sitemap.xml` is an index of per-locale pairs (`sitemap-pages.xml`/`sitemap-jobs.xml` ×
13 locales); the same ~460 postings are mirrored across locales in different languages.
Use only `https://www.brunel.net/en/sitemap-jobs.xml` (the English/global locale) —
`resolveSubSitemap(ctx, c, indexURL, "en/sitemap-jobs")` picks it out of the index — so
the catalogue gets one copy of each posting rather than up to 13 near-duplicates. Each
posting's `JobPosting` ld+json (inside a `@graph` array alongside `WebPage`/
`Organization`) carries title/description/dates/location/salary; same dataart shape past
that point.

## NES Fircroft (nesfircroft.com)

The real client-vacancy catalog is `https://www.nesfircroft.com/job-search/` (824
postings) via the Vennture platform's shared gateway, NOT `careers.nesfircroft.com` (84
postings — confirmed to be NES Fircroft's own internal hiring, a different Vennture
tenant on the same shared API). The gateway resolves which tenant's data to serve from
the `Origin`/`Referer` header on the `/auth` call, not from a request parameter:

1. `GET https://gateway.wearevennture.co.uk/auth?session=&user=` with
   `Referer: https://www.nesfircroft.com/` → an anonymous JWT scoped to the
   `nes-fircroft` tenant (824 jobs), as opposed to the `nes-fircroft-careers` tenant (84
   jobs) a request without that header — or with the careers subdomain's — resolves to.
2. `POST https://gateway.wearevennture.co.uk/job-search?local=uk&organisation=` with
   `Authorization: Bearer <jwt>` and `{"pageSize":50,"nextPageToken":"..."}` → paginated
   JSON (`id, slug, url, title, description, postDate, expiryDate, location, salary*,
   jobTypes, sector, consultant, company.name`), `totalCount` in the response.

This is the one adapter in the batch that needs a header the shared `Client` doesn't
send by default. Use the existing `HeaderJSONGetter`-style mechanism (`http.go` already
documents "custom headers never override the standard User-Agent/Accept") to attach
`Referer: https://www.nesfircroft.com/` on both the `/auth` and `/job-search` calls — no
new transport needed, same shared client, just a header.

Pagination shape matches `emagine.go`'s `PostJSON` + `skipCount`-style loop, adapted to
this API's `nextPageToken` field (stop when the response carries no further token, or
`len(jobs) >= totalCount` as the backstop, same role `emagineMaxPages` plays there).

## Shared decisions

- All four are `boardless()`, none are `aggregator()` — `Company` is a constant string
  per adapter (`"Orion Group"`, `"Airswift"`, `"Brunel"`, `"NES Fircroft"`), matching the
  confirmed-anonymized end client. No new company-resolution mechanism anywhere.
- No crawler UA, no pacing needed for any of the four — confirmed live, no bot defense
  observed on any of them.
- Build order: Orion → Airswift → Brunel → NES Fircroft (simplest/most self-contained
  first; NES Fircroft last since its header requirement is the one genuinely new
  mechanism in this batch). Each ships as its own OpenSpec change, its own branch, its own
  PR, reviewed before merge — not one combined change, since the four share only the
  high-level rationale above, not any code.

## Out of scope

- Petroplan — dead site, not resurrectable without them rebuilding their vacancy engine
  (or this source migrating to `txmgroup.com`, unverified).
- Any change to `bayt.go`/`gulftalent.go`/the `aggregator` marker — this batch doesn't use
  it.
- A shared "agency JSON API" base type across Orion/NES Fircroft — the two APIs differ
  enough (GET+query-param vs POST+JSON-body+header, different pagination fields) that a
  shared abstraction would cost more than it saves for two call sites.
