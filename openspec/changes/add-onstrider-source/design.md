## Context

Strider (onstrider.com) runs its careers site on HubSpot CMS. A spike established:

- The listing page `/jobs` server-renders only ~12 "featured" jobs and ignores `?page=N` — it
  is **not** a usable enumeration source.
- `https://www.onstrider.com/sitemap.xml` enumerates **all** vacancy URLs
  (`/jobs/<slug>-<8hex>`, ~296 at spike time) plus blog/marketing/localized/`preview-slug`
  URLs. Closed vacancies are **retained** in the sitemap and still return HTTP 200.
- Each *open* vacancy page carries a schema.org `JobPosting` ld+json block; *closed* vacancies
  drop the markup (Google penalizes stale `JobPosting` structured data). So **JobPosting
  presence is the open/closed signal.** In a 25-URL spike sample only 3 were open.
- `hiringOrganization.name` is always `"Strider"` — the real employer is hidden until a
  candidate is matched.
- The site sits behind Cloudflare; a plain browser UA passed from a residential IP, but the
  prod datacenter IP is untested and may be blocked (as djinni/eightfold are).

This is the exact DataArt shape (`internal/sources/dataart.go`): sitemap enumerate →
per-vacancy detail fetch → map `JobPosting` ld+json, over the shared `fetchDetails` +
`ldJobPosting` helpers.

## Goals / Non-Goals

**Goals:**
- A keyless, boardless, single-company `onstrider` adapter that emits only open vacancies.
- Reuse the existing sitemap + ld+json machinery; no new helper unless a field shape forces it.
- Enroll in `proxiedProviders` so a blocked prod IP is a config flip, not a code change.

**Non-Goals:**
- Recovering the hidden real employer (not exposed anywhere pre-application).
- A dedicated liveness probe. Closed vacancies simply stop being emitted; the standard 48h
  ingest sweep soft-closes them. `cmd/liveness` would be useless here anyway (closed pages 200).
- Aggregator/ATS suppression — every posting is company `Strider`, so there is no first-party
  ATS twin to dedup against.

## Decisions

- **Enumerate via sitemap, filter to canonical `/jobs/<slug>-<8hex>`.** A single anchored regex
  keeps canonical vacancy URLs and drops blog, marketing, `/pt/`+`/en/` localizations, and
  `/preview-slug-<uuid>/...` duplicates. *Alternative rejected:* crawling `/jobs` — it only
  exposes ~12 featured jobs with no pagination.

- **Open/closed = `JobPosting` presence, obtained for free from `ldJobPosting`.** Like DataArt,
  `detail()` returns `ok=false` when `ldJobPosting(root, &p)` finds no `JobPosting` block, so
  closed vacancies are dropped with no extra logic. *Alternative rejected:* `validThrough` —
  the spike found it unreliable (equals `datePosted`, often long past on still-open jobs).

- **Map the onstrider-specific `JobPosting` shape.** Its fields differ from djinni's, so the
  decode struct is bespoke:
  - `identifier` is a `PropertyValue` object → read `identifier.value` (a UUID) as `ExternalID`.
  - `applicantLocationRequirements` is an **array** of `{"@type":"Country","name":"BR"}` →
    join the `name`s as the location.
  - `employmentType` is an **array** (`["PART_TIME","CONTRACTOR"]`) → normalize via the existing
    `schemaEmploymentType` over the joined/first value.
  - `jobLocationType: "TELECOMMUTE"` → `Remote: true`, work mode `remote`.
  - `hiringOrganization.name` ignored for the company; company is the fixed board-file `Strider`.

- **Description handling captured from a real fixture.** The detail description is HTML in the
  ld+json; sanitize with `sanitizeHTML(html.UnescapeString(...))` as DataArt does. The TDD
  fixture is a saved real vacancy page, so the exact escaping is verified against ground truth
  rather than assumed.

- **Boardless single-company, keyless.** Config entry is `company: Strider` with no board, like
  DataArt. One line in `sources.All`; one entry in `proxiedProviders`.

## Risks / Trade-offs

- **Fan-out cost: ~296 detail fetches per run to surface ~12–40 open jobs.** → Reuse
  `fetchDetails` with the default worker pool (as DataArt/EPAM do); the sitemap set is small
  enough that a throttled fan-out stays within the cron window. A future optimization (probe the
  HubDB public API for a status column in one call) is noted but out of scope.
- **Prod datacenter IP may be Cloudflare-blocked.** → Enroll in `proxiedProviders` up front so
  the crawl egresses through `SOURCES_PROXY_URL` when set; if the direct IP turns out to work,
  the proxy is simply not configured. Same posture as djinni.
- **Single company facet.** All jobs collapse under `Strider`, so company-level facets are
  coarse. Accepted — the real employer is genuinely unavailable.

## Migration Plan

- No DB migration, no API change, no new dependency.
- Deploy: add a `cmd/ingest sources/onstrider.yml` cron schedule in freehire-ops; set
  `SOURCES_PROXY_URL` if the prod IP is blocked (verify with one manual run first).

## Open Questions

- Does the prod datacenter IP actually get Cloudflare-blocked? Resolve with a single manual prod
  run; `proxiedProviders` enrollment makes the answer a config flip either way.
