## Context

See proposal.md - Why. All of the following was verified live on 2026-09-06 against the
production host and is written up for `internal/ingest/sources/AGENTS.md` as part of this change
(task 1), in the same "`<Provider>` traps" style as the Dayforce/Workstream/EDJOIN entries already
there.

**Vendor and hosts.** `apply.jobappnetwork.com` serves talentReef's applicant-facing React SPA
(`<title>talentReef</title>` in the served HTML). The SPA's own bundle names the real API hosts:
- `https://prod-kong.internal.talentreef.com/apply` — the REST API (its "internal" name is
  misleading: it resolves publicly and answers unauthenticated requests from any origin,
  `access-control-allow-origin: *`).
- `.../apply/proxy-es` — a proxy in front of the listing's Elasticsearch index.

**The listing call.** `POST https://prod-kong.internal.talentreef.com/apply/proxy-es/search-en-us/posting/_search`,
body `{"query":{"bool":{"filter":[{"term":{"clientId": <id>}},{"term":{"internalOrExternal":"externalOnly"}}]}},"from":<offset>,"size":<n>}`.
Confirmed live: filtering on `clientId` alone returns exactly that employer's postings (tested
against client 20448, "HZ Coffee Group LLC", 2729 hits, and client 10681, talentReef's own
dogfooding board, 1 hit). The response is a plain Elasticsearch hit list —
`hits.total` (exact, confirmed stable across page sizes) and `hits.hits[]._source`.

**Locale.** The index name (`search-en-us`) is the platform's locale slice; `search-fr-ca` and
`search-es-us` exist for the same postings translated. English-only for this change, matching the
issue's scope and every other adapter's default-locale convention (e.g. Dayforce's
`dayforceDefaultCulture`).

**Posting shape** (fields actually observed; irrelevant styling/tracking fields omitted):
```json
{
  "jobId": 7216553,
  "title": "Shift Leader",
  "description": "<p>...</p>",
  "category": "Restaurant Staff",
  "address": {"street1": "...", "city": "Houston", "stateOrProvince": "TX", "country": "US"},
  "clientId": 20448,
  "clientName": "HZ Coffee Group LLC",
  "createdDate": "2022-02-16",
  "internalOrExternal": "externalOnly",
  "url": "/apply/c_mdm/l_en/Shift-Leader-job-Houston-TX-US-7216553.html"
}
```
`description` is the full posting body (rich HTML), not a preview — no separate detail request is
needed, the same shape Dayforce's listing carries.

**A discovered risk, out of this change's scope to fix.** The ES proxy is multi-tenant and only
weakly scoped by the query the caller sends — a `match_all` query with no filter returns postings
across every talentReef client on the platform, and one indexed document sampled during this
research was clearly injected by an unrelated third party ("you are hacked"-shaped content), which
is evidence the cluster has been probed/abused before. This is the vendor's exposure, not
freehire's, and is irrelevant to correctness here as long as this adapter always sends the
`clientId` term filter — but it is why the request must never be built from unscoped input.

**Employer shape.** One `clientId` is one talentReef account (a company or a franchise operator
running many locations under one or more brands) — a single-employer board, not an aggregator:
every posting sampled under one `clientId` shared the same `clientName`. This matches Dayforce's
and Gusto's model (`Company` comes from the board's configured company), not Crelate's
(aggregator, company read per posting).

**Catalogue fit.** talentReef's customer base skews heavily toward hourly/frontline retail and
restaurant hiring (the sampled clients are Dunkin' and Baskin-Robbins franchisees), but the
platform is not exclusively that — the same index holds postings for at least one corporate/tech
account (a "Senior DO-178 Software Engineer" at an aerospace client, a "Software Quality Assurance
Engineer" at a furniture retailer's corporate office). This is exactly the shape
`internal/ingest/pipeline/catalogue_fit.go` exists for ("a generic ATS board carries a company's
whole hiring, not its engineering org") and needs no special handling beyond the existing
non-tech-title rejection every crawled board already goes through.

## Goals / Non-Goals

**Goals:**
- Crawl a single jobappnetwork board (client id) to the same standard every other single-employer
  adapter meets: paginate to the platform's own exact total, map every field the listing states,
  never fabricate one.
- Never surface a posting the platform itself marks internal-only.
- Recognize the platform's real public URL shape so the contribution/harvest flow can onboard a
  board without a person naming the provider by hand.

**Non-Goals:**
- Alias-based onboarding (`/careerPages/alias/<name>` → clientId). Every apply link the harvest
  actually sees already carries the numeric clientId directly (issue #2080's own sample), so
  resolving a human-readable alias adds a request and a second board-identity question
  (alias and clientId are two spellings of the same employer) for a case this change never hits.
  If a future harvest surfaces alias-only links, add it then.
- Non-English locales (`search-fr-ca`, `search-es-us`). Out of scope per Dayforce's precedent and
  the issue's own framing (rank by yield, ship the highest one first).
- `gr8people`/`workgr8.com` and `werecruit.io` — the issue's next two platforms, explicitly
  deferred to their own changes.

## Decisions

**Register the host in `atsboard`'s `apiBoards` table, not a new `atsBoards` mode.**
`apply.jobappnetwork.com/clients/<id>/posting/<postingId>/…` is mechanically identical to what
`apiBoards`/`recognizeAPI` already does for Ashby/Greenhouse/Lever: a fixed path prefix
(`"clients"`) followed immediately by the board segment, with everything after it ignored. Adding
a one-off `atsBoards` mode to do the same thing would duplicate `recognizeAPI`'s logic for no
gain — `apiBoards`'s doc comment describes its usual case as "the XHR a career site... makes", but
nothing about the mechanism requires that; a person-pasted URL of the same shape resolves exactly
as correctly. Alternative considered: a new `modePathAfterLiteral` mode in `atsBoards` — rejected
as pure duplication of `recognizeAPI` with no behavioral difference.

**Filter `internalOrExternal: externalOnly` in the query itself, not after the fact.** The field
is either `externalOnly` or `internalOnly` on every sampled posting. Filtering server-side (a
second `term` clause alongside `clientId`) means an internal posting never crosses the wire and
never has to be remembered as "returned but dropped" in the mapping code — the same reasoning
Dayforce's culture handling and EDJOIN's required-parameter handling both apply: push a platform
distinction into the request when the platform accepts it as a filter, rather than into
after-the-fact code that a later change could forget to call.

**Not a `HydratingSource`.** The listing carries the complete `description` HTML, confirmed
across every sampled posting — no truncation marker, no separate detail endpoint referenced by the
SPA for the listing's own postings. Matches Dayforce's listing, not Workstream's or Gusto's
preview-only one.

**Board validation: require a positive integer, reject everything else.** The platform's own URLs
never carry anything else in that segment (confirmed against both sampled clients, 20448 and
10681), and a non-numeric board could otherwise be sent straight into a query filter expecting a
number, producing a platform-side error that reads like a transient failure rather than a
configuration mistake.

**Pagination via plain Elasticsearch `from`/`size`, stopping on `hits.total`.** `hits.total` was
exact and stable in every sample taken (matches the "exact total" family — Dayforce's `maxCount`,
EDJOIN's `totalRecords` — not SEEK's page-size-dependent `totalCount`). The result window this
implies (Elasticsearch's default 10,000-document cap on `from+size`) is not reachable by any
single client observed (the largest sampled, 2,729) and is far above what a single-employer board
on this platform plausibly holds, so no `search_after` handling is needed for this change.

## Risks / Trade-offs

- **[Undetermined rate limiting]** No adapter-scale request volume was sent against the endpoint
  during this research (only a handful of manual probes) — unlike the "traps" entries for other
  providers, this change does NOT claim "no metering observed". → Ship without a pacer (matching
  most adapters' default), and watch the first real board-catalog crawl's `board_health` rows for
  429s/failures before onboarding many boards at once.
- **[Vendor-side multi-tenant exposure]** The listing endpoint is far less scoped than a
  well-designed per-tenant API would be (see Context) — a bug in this adapter that ever dropped
  the `clientId` filter would silently return another employer's postings under this board's
  company. → The filter is a fixed, non-optional part of every request the adapter builds; tests
  assert the request body always carries it.
- **[Catalogue yield may be thin per board]** Per Context, most of any one client's postings will
  be frontline/hourly and rejected by `catalogue_fit`. → Accepted; this is the same trade every
  generic-ATS adapter already onboarded makes, and boards are onboarded one company at a time via
  the harvest, not as a whole-platform crawl (unlike EDJOIN/SchoolSpring, where that distinction
  had to be a design decision — here it already is the shape the issue asks for).
