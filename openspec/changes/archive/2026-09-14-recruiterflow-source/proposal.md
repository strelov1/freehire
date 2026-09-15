## Why

RecruiterFlow (`recruiterflow.com/<board>/jobs`) is a real multi-tenant recruiting-agency
career-page platform found while draining `board_submissions` (id 26,
`recruiterflow.com/radhires/jobs/369`). An earlier drain pass left it as an unbuilt lead
because the listing page is a legacy jQuery SPA with no server-rendered job links, no
schema.org ItemList, and no discoverable REST endpoint from static HTML/JS inspection.
Re-investigating with the raw page HTML (rather than only looking for `__NEXT_DATA__`/
RSC-flight/ld+json shapes) found the data was there all along, embedded as a plain
JavaScript variable assignment (`window.jobsList = {...};`) — no XHR, no headless browser,
no API discovery needed at all.

## What Changes

- Add a `recruiterflow` source adapter (`internal/ingest/sources/recruiterflow.go`).
- Listing: fetch the board's `/jobs` page as raw text and extract the `window.jobsList =
  {...};` JS object via the existing shared `bracketSlice` primitive (the same
  balanced-brace/string-aware extractor `nextflight.go` already provides). Its `department`
  key groups every open posting by department; flattening across every group yields the
  whole board — id, title, location (a free-text region list), employment type, remote
  type, and last-opened date, everything except the description.
- Detail: each posting's own page (`recruiterflow.com/<board>/jobs/<id>`) embeds a standard
  schema.org `application/ld+json` `JobPosting` block, reused via the shared `ldJobPosting`
  decoder — the same one `geekhunter`/`recrutei` already use — read only for the
  description.
- Confirmed live: `hiringOrganization.name` in every sampled posting's ld+json is the
  agency itself ("Rad Hires"), never a per-posting end-client name — the same "the board's
  own configured name is the only company signal" shape `huntflow`'s agency/hub boards
  already have, not a special case to design around.
- Register `recruiterflow` in `sources.All`. **No `internal/ingest/atsboard` recognizer
  entry** — `recruiterflow.com` is a bare apex domain the platform's own marketing site
  also lives on (`recruiterflow.com/pricing`, `/blog`, etc. all answer 200), and the
  shared recognizer table has no mechanism to require a board be paired with the `/jobs`
  path segment every real tenant URL carries; adding a plain path-mode entry would
  misrecognize a marketing page as a brand-new board. See design.md.
- Register `recruiterflow/radhires` via `cmd/add-board` once merged and deployed, closing
  `board_submissions` id 26.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `source-ingest`: add a requirement that `recruiterflow` is a registered board-based
  provider — a single-page listing (embedded as a JS variable, not fetched separately) plus
  per-posting ld+json detail-hydration adapter over `recruiterflow.com/<board>`, yielding
  the normalized job shape including structured employment type and work mode from the
  listing's own fields.

## Impact

- New files `internal/ingest/sources/recruiterflow.go` (+ test).
- `internal/ingest/sources/registry.go`: one new registration line.
- No `internal/ingest/atsboard` changes — see What Changes.
- Frontend source-facet enum (`contracts.ts`) regenerated to include `recruiterflow`.
- No changes to `internal/ingest/sources/nextflight.go`/`jsonld.go` — this PR only consumes
  the existing `bracketSlice`/`ldJobPosting` primitives.
