## Context

`dover` went live as a crawled source (add-dover-ats-adapter) with boards addressed by the
company's Dover slug — the segment of `app.dover.com/apply/<slug>/<id>` and
`/jobs/<slug>`. The adapter resolves that slug to Dover's internal client UUID via
`GET /api/v1/careers-page-slug/<slug>` and then pages `GET /api/v1/careers-page/<uuid>/jobs`.

Dover also serves a dedicated public job-board host, `jobs.dover.com`, built on the same
search-builder SPA as the apply pages. It addresses a company's board as a two-segment pair:

    https://jobs.dover.com/<Org>/<client-uuid>
    e.g. https://jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d

with optional deeper segments for a posting. The first segment is the company's slug; the
second is the client UUID — the exact identity the careers API lists under.

## Live verification (2026-11-05)

- `GET https://app.dover.com/api/v1/careers-page-slug/Deepspatial` → 200, returns
  `id: 1aeeba55-65e0-4803-b70f-fac7ca8a20c7, slug: deepspatial, name: Deepspatial`.
- `GET .../careers-page/1aeeba55-…/jobs?limit=10&offset=0` → 200, `count: 4` with real
  postings (AWS Architect, Full Stack Developer, …).
- `GET .../inbound/application-portal-job/a5546acf-…` → 200, full detail of the submitted
  posting, `client_id: 1aeeba55-…` — the pair's uuid is the client id of the same company.
- `GET https://jobs.dover.com/Deepspatial` (bare, no uuid) → 200: the marketing shell serves
  ANY slug, so a bare `/<Org>` proves nothing and must not be read as a board.
- `GET https://jobs.dover.com/startup-jobs` → the vendor's own curated listing, no tenant.

## Goals

- A `jobs.dover.com` link resolves to `(dover, <Org>/<client-uuid>)` with a canonical that
  collapses to the two-segment board address, so a posting link and the board listing map to
  one crawlable board — the same collapse every recognizer mode makes.
- The existing adapter crawls such a board without a slug lookup: the pair carries the client
  id itself.

## Non-goals

- No new provider, no applyform work, no schema changes.
- No support for bare `/<Org>` links: the shell serves any slug, so the segment is not a
  provable board id; such links stay unrecognized (they fall to manual review like any other
  unrecognized link).

## Decisions

**Recognizer mode: `pathpattern`, not a second `allowedfirstsegment` row.** The first attempt
used `modeAllowedFirstSegment` with a synthetic `jobs` key segment — but that host has no key
segment: the pair starts at the root, and faking one would have broken `app.dover.com`'s own
collapse (its posting path is `/apply/<slug>/<jobId>`, where everything after the board is the
POSTING id, never part of the board). `pathpattern` fixes both the position and the shape in
one regex: `/<Org-slug>/<uuid>` with an optional trailing posting slug left out of the capture.
The UUID shape in second position is what separates a board from the SPA's other pages — a
bare `/<Org>` and the vendor's `/startup-jobs` carry no second segment and are declined by the
pattern itself, with no `reject` list needed.

**Board id keeps the slug AND the uuid (verbatim, not folded).** The uuid is load-bearing —
it is the identity the listing endpoint pages under — and the slug keeps the board legible in
the catalogue and disambiguates two Dover clients that could otherwise only be told apart by
uuid. The capture is kept as the link spelled it; Dover serves mixed-case orgs (the API's own
`slug` field is lower-case while the host path is `Deepspatial`), and folding would split one
board into two catalogue rows spelled differently.

**Adapter: a pair short-circuits the slug lookup.** `resolveClientID` detects
`<anything>/<client-uuid>` by shape and returns the uuid. The alternative — resolving the
pair's slug through `careers-page-slug` and checking it matches the uuid — costs a request on
every crawl, and the marketing shell resolves ANY slug, so agreement would prove nothing; the
uuid in the URL is the stronger fact. A plain slug (no slash, or a slash whose second segment
is not a uuid) keeps the existing lookup untouched.

## Risks

- A Dover client uuid could theoretically be reused across hosts — it cannot: the same uuid
  resolves on `app.dover.com`'s API, which is the only API the adapter calls.
- Boards harvested as pairs and boards harvested as slugs can coexist for the same company as
  two catalogue rows (`deepspatial` vs `Deepspatial/1aeeba55-…`). That is the same
  spelling-variant situation every URL-derived board id lives with; dedup happens at the job
  level (`UNIQUE (source, external_id)`), so postings are not duplicated — only two crawl
  slots are spent.
