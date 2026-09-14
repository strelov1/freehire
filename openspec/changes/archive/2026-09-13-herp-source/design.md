## Context

See proposal.md for the discovery and the live investigation it's based on. HERP
(`herp.careers`) is a Japanese ATS with no public listing API — every page is server-rendered
HTML — but each job page's `application/ld+json` `JobPosting` block is standard schema.org,
identical in shape to what `internal/ingest/sources/breezy.go` and `teamtailor.go` already
decode via the shared `ldJobPosting` helper, and listing discovery reuses the shared `jobLinks`
DOM-walking helper (`internal/ingest/sources/html.go`) already used by several HTML-listing
adapters.

## Goals / Non-Goals

**Goals:**
- Discover a board's jobs from its listing page and any requisition groups it links to, one
  level deep.
- Reuse existing shared helpers (`jobLinks`, `ldJobPosting`, `fetchDetails`) rather than writing
  new DOM-parsing primitives.
- Satisfy the `fullBoardListing` contract, so the pipeline's unseen-sweep can safely close a
  posting HERP no longer lists.

**Non-Goals:**
- A bespoke `cmd/harvest-boards` prober. `herp` has no cheap count endpoint (unlike, say,
  `joinProber`'s `rowCount`) — the only way to check liveness is to run the same listing walk
  the adapter itself does, which is exactly what `adapterProber`'s fallback already provides
  for a board-keyed provider with no bespoke entry in `probers`. Writing one anyway would just
  reimplement `Fetch` inside `cmd/harvest-boards`.
- Deeper nesting than one level of requisition groups, or listing pagination. Live inspection
  found neither; if a company's true catalogue turns out to need one, that surfaces as a
  measurably shrunken board and gets its own follow-up fix — the same pattern this codebase
  already uses for `apploi`/`bayt`/`teamtailor`'s own `-fullboardlisting-fix` corrections,
  rather than something to guess at upfront.
- The `herp-react-props` JSON blob embedded in each job page. It duplicates the same fields
  the page's own `application/ld+json` block already exposes in the exact shape the shared
  `ldJobPosting` helper reads — decoding it would be a second, redundant path to the same data.

## Decisions

**`atsboard` mode: `path`, with `v1` reserved.** A board sits at `/v1/<board>/…` — `v1` is
platform machinery, never a tenant, so it goes in `reservedSegments["herp.careers"]` exactly
like Gusto's `/boards/<board>` reserves `"boards"`. No new extraction mode is needed.

**Link matching is host+path-shape, never substring.** `jobLinks`'s `isJob` predicate receives
the raw, unresolved `href` text — a share-widget link (`https://twitter.com/share?url=<url-encoded
herp.careers link>`) would match a naive substring test. The adapter resolves each href to a
`*url.URL`, checks the host is empty (relative) or exactly `herp.careers`, and only then reads
the path shape — the same defensive parse `atsboard.Recognize` uses everywhere else in this
codebase.

**One level of group expansion, deduplicated once at the end.** The company page and every
requisition-group page each run through `jobLinks` independently (it dedupes within one page),
so the aggregated result across pages is deduplicated again before detail-fetching — the same
job can appear both directly on the company page and inside one of its groups.

**`ExternalID` = the job link's final path segment** (HERP's own opaque per-posting id, e.g.
`GnoQonoXGBZi`) — stable, unique per board, and exactly what the URL already encodes; no
separate id field needs reading out of the ld+json block (schema.org `JobPosting` doesn't
carry one).

**Location and WorkMode are left to the pipeline's own dictionary.** The ld+json
`jobLocation.address` is a free-text Japanese address, not a structured remote/on-site flag —
consistent with the `Job.WorkMode` contract (structured signal only), this adapter leaves
`WorkMode` empty and lets the location-text fallback matcher run, the same choice
`breezy`/`teamtailor` make for a platform with no structured field.

## Risks / Trade-offs

- **A company whose true catalogue needs a second level of group nesting, or listing
  pagination, would under-count until observed.** Accepted per Non-Goals above — this mirrors
  how several other HTML-listing adapters in this codebase were shipped and then corrected once
  a real crawl showed a cap.
- **No bespoke prober means `cmd/harvest-boards` pays a full crawl per candidate when
  discovering new HERP boards**, the same cost profile Cornerstone/Taleo-style adapters already
  have via the same fallback. Acceptable for HERP's inventory size (~970 companies).
- **Platform navigation sharing the job link's own host and path shape is a real, demonstrated
  risk, not a hypothetical one.** Code review caught that a board's optional `/v1/<board>/top`
  landing page — real, live, and present on a measurable share of HERP companies — passed the
  original host+path check exactly like a real job link would, and its detail page (no
  `JobPosting` block) would have been marked Unreadable on every single crawl. Because
  `internal/ingest/pipeline`'s stale-job close withholds once a board's Unreadable share crosses
  a small threshold, this was not a one-time miss but a PERMANENT loss of that board's ability
  to detect a closed posting. `herpNonJobSegments` excludes the literal `top` segment, the same
  reserved-word pattern this codebase already uses for Gusto's `boards` and Jobvite's `careers`.
  The lesson for any FUTURE `atsboard`/adapter addition: "host+path shape, never substring"
  guards against an unrelated host, not against the platform's OWN machinery living at the same
  host and path depth as a real tenant link — that still needs an explicit reserved-word check.
