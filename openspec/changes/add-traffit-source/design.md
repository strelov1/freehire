## Context

Traffit hosts each client on `<tenant>.traffit.com`. The career widget bootstraps from
`/public/an/generateJs/` and fetches postings from `/public/an/list/`. That list endpoint
is keyless JSON and self-contained — the description HTML is inline, so no per-posting
detail fetch is needed. Verified during source discovery:

- `GET https://<tenant>.traffit.com/public/an/list/` → `{"count":N,"items":[...]}`.
- A non-tenant subdomain (wildcard DNS + wildcard cert make every subdomain resolve)
  returns an HTML placeholder, not JSON.
- Page size caps at 100 (`?limit`), defaults to 10; `?offset` works. So large boards
  MUST be paged (e.g. `jit` = 167 postings).
- 14 live tenants validated (~385 open postings): cloudfide, traffit, trust, people,
  spline, zen, welove, jit, balticamadeus, fintalent, b2bnetwork, soflab, itlt, hrhub.

Relevant item fields: `advertId` / `advertPublishId` (stable id), `title`/`name`,
`description` (HTML), `geolocation` (a JSON **string** with `locality`/`region1`/
`country`/`iso`), `language`, `url`, `applicationForm`, `validStart` (Unix seconds).

## Goals / Non-Goals

**Goals:**
- A keyless `traffit` adapter that returns every live posting of one tenant, paged.
- A harvest prober that live-validates candidate tenant slugs.
- A seeded `sources/traffit.yml` with a validated starter tenant set.

**Non-Goals:**
- Auto-discovery of tenants (not keyless-feasible: wildcard DNS/cert; crt.sh only sees
  marketing subdomains). Seed comes from search dorks / a curated list, validated live.
- Applicant-form parsing (`/public/form/...`) — that page is a dead end for ingest.
- A central Traffit aggregator — none exists (`jobs.`/`kariera.` return 503).

## Decisions

- **Board = tenant subdomain; board-based, not boardless.** Mirrors greenhouse/lever:
  one entry per tenant, `board` required, stays in the source facet. `Provider()` →
  `"traffit"`.
- **Pagination loop.** Request `?limit=100&offset=N` starting at 0, accumulate items,
  stop when `len(collected) >= count` or a page returns 0 items. Cap the loop with a
  page bound (defensive, like gupy's `gupyMaxOffset`) so a misbehaving feed can't spin.
- **Dedup id = `advertId`** (fall back to `advertPublishId` if absent), stringified.
  Skip a posting with no id (would collide on the dedup key), matching comeet.
- **Location** from the `geolocation` JSON string: parse it, render `locality` (+
  `region1`/`country` when present). `geolocation` can be `null` (e.g. cloudfide's first
  posting) → empty location, the dictionary derivation stays silent, no guess.
- **posted_at** from `validStart` via the existing `parseEpochSeconds` helper.
- **Description** run through the shared `sanitizeHTML`.
- **WorkMode** left empty — Traffit's list carries no structured workplace enum, so the
  pipeline's location/dictionary derivation decides (no free-text heuristic in the adapter,
  per the structured-signal convention).
- **Prober** (`cmd/harvest-boards/traffit.go`): `GetJSON` the list endpoint for a slug;
  on error (HTML placeholder won't parse) or zero items return `("", 0, nil)` — a skip.
  Company name falls back to the slug; a human curates display names in `sources/traffit.yml`.
  Registered in `probers["traffit"]`. Not a `discoverer` (no keyless enumeration source).

## Risks / Trade-offs

- **Seed staleness / manual discovery.** Without auto-discovery, new tenants are found
  only when someone re-runs a dork sweep and re-probes. Acceptable: the prober makes
  re-validation cheap, and the adapter's value doesn't depend on exhaustive coverage.
- **Page-size assumption.** The 100 cap is observed, not documented; if Traffit changes
  it the loop still terminates on the `count`/empty-page conditions, only the request
  count changes.
- **Language mix.** Many tenants post in Polish; that's expected for CEE coverage and the
  enrichment/classification dictionaries already handle non-English input.
