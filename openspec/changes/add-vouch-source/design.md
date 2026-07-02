## Context

vouch.careers is a Next.js App Router app. A company's jobs live at
`vouch.careers/companies/<companyId>` (companyId is a cuid, e.g.
`cmghojtua00p5et0dvusuxx5o`). A spike established:

- The company page **server-renders every posting into the RSC flight** (a sequence
  of `self.__next_f.push([1,"…"])` chunks). Concatenating and JSON-unescaping the
  chunks yields a flight string that contains a clean `"listings":[ {…}, … ]` JSON
  array — one object per posting.
- Each listing object is fully populated: `id` (cuid), `url` (relative detail URL),
  `title`, `pitch`, `must` (requirements HTML), `nice` (nice-to-have HTML),
  `description` (often empty), `employmentType` (array of tokens like
  `["Full-time","Remote"]`), `company` (`{name, …}`), `locations` (array, often
  empty for remote roles), `publishedAt` (RFC3339), and the state flags `activated`
  / `draft` / `unlisted`.
- Everything is keyless. No per-job detail fetch is needed — the company page has it
  all. The observed board (Laine) returned 11 live listings.

The codebase already parses Next.js flight for Deel (`internal/sources/deel.go`),
including a generic balanced-slice extractor (`bracketSlice`) and a flight decoder
(`decodeDeelFlight`).

## Goals / Non-Goals

**Goals:**
- A board-based `vouch` adapter that returns every live posting for one company from
  a single keyless page fetch.
- Stable dedup identity from the listing cuid; structured work mode from
  `employmentType`; clean HTML description from the body fields.
- Reuse the existing flight primitives rather than re-implementing them.

**Non-Goals:**
- Per-job detail fetches or streaming — one page carries the whole board.
- A harvest prober / company auto-discovery. Only one board is known today.
- Parsing `salary`/`reward` into structured facets — enrichment owns that.
- Resolving the Supabase storage/image URLs or company-level `company-locations`
  block — a listing's own `locations` (when present) is the location source.

## Decisions

- **Extract everything from the single company page's flight.** Fetch
  `https://vouch.careers/companies/<board>`, decode the flight, `bracketSlice` the
  `"listings":` array, and `json.Unmarshal` it into `[]vouchListing`. This mirrors
  `extractDeelPostings` but without Deel's text-row indirection (vouch inlines the
  body HTML directly in each listing, so no `$N` reference resolution is needed).

- **Reuse `bracketSlice`; promote the flight decoder to a shared name.**
  `bracketSlice` is already generic and stays as-is. `decodeDeelFlight` and its
  `deelPush` regexp are generic Next.js-flight mechanics; rename them to
  `decodeNextFlight` / `nextFlightPush` (updating Deel's single call site) so a
  second consumer isn't coupled to the "deel" name. No behavior change; deel's tests
  cover the rename.
  - *Alternative considered:* call `decodeDeelFlight` from `vouch.go` unchanged.
    Rejected — it reads as vouch depending on deel; the neutral name is the honest
    shape now that the primitive is shared.

- **Only live postings.** Emit a listing only when `activated && !draft &&
  !unlisted`. The `listings` array can carry hidden/draft rows that are not real
  vacancies.

- **ExternalID = listing `id`** (cuid), with the standard empty-id drop-guard. **URL**
  = the listing's `url` resolved against `https://vouch.careers`. **Company** =
  `firstNonEmpty(listing.company.name, e.Company)`.

- **Description** = the body fields joined as HTML — `pitch` + `must` + `nice` — then
  `sanitizeHTML`. `description` is usually empty, so the real content is those three.

- **Work mode (structured):** map `employmentType` tokens — "Remote"→remote,
  "Hybrid"→hybrid, "On-site"→onsite (first match wins) — into `WorkMode`, and set
  `Remote` when the tokens contain "Remote" (or the location heuristic matches). This
  is a structured field, so it is authoritative (provenance stays clean).

- **Location** from the listing's own `locations` array (city/country when present),
  else empty — remote roles legitimately have no location.

- **Transport interface = `HTMLGetter`** (one HTML page).

## Risks / Trade-offs

- **RSC flight is version-sensitive** (a Next.js upgrade can reshape the stream) →
  the adapter errors loudly when `listings` is absent (never silently empties), and a
  table-driven test over a captured flight fixture pins the shape. This is the same
  bargain the Deel adapter already accepts.
- **`employmentType` token vocabulary drift** → an unrecognized token yields empty
  work mode (the location parser/LLM still resolve remote), never a wrong value.
- **A company with a very large flight** (many postings) is one big response → still
  a single fetch, far cheaper than a per-job fan-out; acceptable.

## Migration Plan

Additive and read-only: new adapter file, one registry line, one board file, plus a
no-behavior-change rename in deel.go. No schema/API/migration changes. Rollback =
remove the registry line / board file.

## Open Questions

- None blocking. Company auto-discovery (a prober) is deferred until the board list
  justifies it.
