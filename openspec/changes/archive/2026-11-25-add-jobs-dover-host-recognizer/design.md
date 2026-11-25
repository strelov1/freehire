## Context

`dover` went live as a crawled source (add-dover-ats-adapter) with boards addressed by the
company's Dover slug — the segment of `app.dover.com/apply/<slug>/<id>` and
`/jobs/<slug>`. The adapter resolves that slug to Dover's internal client UUID via
`GET /api/v1/careers-page-slug/<slug>` and then pages `GET /api/v1/careers-page/<uuid>/jobs`.

Dover also serves a dedicated public job-board host, `jobs.dover.com`, built on the same
search-builder SPA as the apply pages. It addresses a company's board as:

    https://jobs.dover.com/<Org>/<uuid>      e.g. /Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d

with optional deeper segments for a posting. `<Org>` is the company's Dover slug — the same
identity the `app.dover.com` paths use.

## Live verification (2026-11-05, re-verified during review round 1)

- `GET https://app.dover.com/api/v1/careers-page-slug/Deepspatial` → 200, returns
  `id: 1aeeba55-65e0-4803-b70f-fac7ca8a20c7, slug: deepspatial, name: Deepspatial`.
- `GET .../careers-page/1aeeba55-…/jobs?limit=10&offset=0` → 200, `count: 4` with real
  postings, among them `a5546acf-…`.
- `GET .../inbound/application-portal-job/a5546acf-…` → 200, full detail; its `client_id` is
  `1aeeba55-…`. **The link's uuid is a posting id, not the client id.**
- `GET .../careers-page/a5546acf-…/jobs` → 404.
- `GET https://jobs.dover.com/Deepspatial` → 200, and so is
  `https://jobs.dover.com/GarbageCorp99999/a5546acf-…`: the marketing shell serves ANY slug
  and any pair, so NOTHING on the host itself proves existence.
- `GET https://jobs.dover.com/startup-jobs` → the vendor's own curated listing, no tenant.

## Goals

- A `jobs.dover.com` link resolves to the same board an `app.dover.com` link for the same
  company spells — `(dover, <Org>)` — so links from both hosts file under one crawl target.
- A link must carry a UUID-shaped second segment to be recognized; a bare `/<Org>` (the
  shell) and the vendor's own pages stay declined.
- The adapter crawls such a board exactly like any other Dover board; posting apply URLs stay
  the platform's own `/apply/<slug>/<jobId>` shape.

## Non-goals

- No new provider, no applyform work, no schema changes.
- No support for bare `/<Org>` links: the shell serves any slug, so the segment is not a
  provable board id; such links stay unrecognized (they fall to manual review like any other
  unrecognized link).

## Decisions

**Recognizer mode: `pathpattern`, not `allowedfirstsegment`.** The host has no key segment —
the board pair starts at the root — and the mode's collapse would have broken `app.dover.com`'s
own posting path (`/apply/<slug>/<jobId>`, where everything after the board is the posting id).
`pathpattern` fixes both the position and the shape: capture group = the slug, followed by a
REQUIRED UUID segment and an optional trailing posting slug left out of the capture. The UUID
shape in second position is what separates a board link from the shell's bare `/<Org>` and the
vendor's `/startup-jobs` (neither carries one), with no `reject` list needed.

**Board id = the slug ALONE; the uuid is never folded in.** The first committed design
captured the pair `<slug>/<uuid>` and assumed the uuid was the client id. Review (four
findings from Claude, Codex ×2 and CodeRabbit, each live-reproducible) disproved that
assumption with the submitted link itself: `a5546acf-…` is a POSTING id — its detail answers
`client_id: 1aeeba55-…`, and the pair-shaped board would have listed
`careers-page/a5546acf-…/jobs` (404), stranding the crawl, while every posted uuid minted a
fresh one-posting board and every apply URL came out malformed
(`/apply/<Org>/<uuid>/<jobId>`). The slug is the identity both hosts already share, so
recognizing to it merges the two spellings into one crawl target instead of duplicating the
company per posting.

**Existence is verified at the API, not at the host.** The shell answers any slug and any
pair (verified with a garbage slug), so the host proves only SHAPE. Existence is checked
where it is provable — `careers-page-slug` resolving the board before its first crawl, the
same validate-on-live-surface step every candidate board goes through. A wrong-slug link that
somehow carried a UUID segment fails its board's first crawl like any non-existent board
would; it never mints phantom rows.

**No adapter change.** Since the board id is the slug, `resolveClientID`'s existing lookup is
the supported address path; the shortcut the interim design added (trusting the pair's uuid)
was removed. The adapter cannot tell which host a board was harvested from and does not need
to — the spelling is identical.

## Risks

- A `jobs.dover.com` link whose slug is stale while its uuid is live (company renamed its
  slug after the link was harvested) would resolve to a dead board and fail its first crawl.
  That is the same failure shape every URL-derived board id lives with (stale links fail
  everywhere in the recognizer's domain), and the uuid cannot rescue it: it is provably a
  posting id on this very link class, so trusting it was the defect under review.
- Case: the host serves `Deepspatial` capitalized while the API's own slug field is
  lower-case. The capture is kept verbatim (pathPatternBoards' default), matching how
  `app.dover.com` links already file boards; the catalogue stores what the link spelled.
