## Context

See proposal.md - Why. Everything below was verified live against the real public API on
2026-09-13 (no key needed), cross-checked against the unofficial community OpenAPI documentation
at https://github.com/rorar/EURES-API-Documentation.

**Search endpoint.** `POST https://europa.eu/eures/api/jv-searchengine/public/jv-search/search`.
Request carries `resultsPerPage` (max 50), `page` (1-based), `sortSearch`, `keywords` (empty array
for an unfiltered search), `publicationPeriod`, `occupationUris`, and a long list of other filter
arrays (all left empty except `locationCodes`), plus `requestLanguage` and a `sessionId` string
(no auth semantics observed — a fixed constant works). Response: `numberRecords` (total matches)
and `jvs[]`, each already carrying `title`, `description` (HTML), `employer.name`, `creationDate`/
`lastModificationDate` (unix ms), `locationMap` (country code → NUTS region codes, no city text),
`positionOfferingCode`, `positionScheduleCodes[]`, and the opaque base64 `id`. When
`requestLanguage` matches an available translation, `title`/`description` are already returned in
that language — no extra step needed.

**Pagination depth cap.** Confirmed live: `page=200` at `resultsPerPage=50` (from=9950) succeeds;
`page=900` fails with `{"errorType":"java.lang.IllegalArgumentException","errorMessage":"Too many
results were requested"}`. This is the same shape as `arbeitsagentur`'s measured ~10,000-result
Elasticsearch `max_result_window`. Measured volumes for the ICT occupation filter (see below) at
various windows, largest-observed segment (Germany, `C25`):

| Window | numberRecords |
|---|---|
| `LAST_WEEK` | 10,528 (over cap) |
| `LAST_THREE_DAYS` | 6,118 |
| `LAST_DAY` | 1,316 |

`LAST_THREE_DAYS` was chosen for comfortable headroom (~39%) under the cap on the largest segment
observed, while still being a short enough window that the adapter's own recency filter, not the
depth cap, is normally what bounds a crawl.

**Occupation filter.** `occupationUris` accepts ESCO occupation URIs, but also accepts the
coarser ISCO major-group URIs directly — confirmed live: `http://data.europa.eu/esco/isco/C25`
("Information and Communications Technology Professionals") alone returned 48,846 records
worldwide (all EURES countries) with titles like "Information Security Analyst" and "Digital
Operations Assistant" (WordPress/WooCommerce), i.e. correctly IT-scoped. `C35` ("ICT Technicians")
is added for the technician-level slice profession.hu's own `itops` category would also capture.
No leaf-level per-occupation enumeration is needed.

**Detail endpoint.** `GET https://europa.eu/eures/api/jv-searchengine/public/jv/id/{id}
?requestLang=en`. Unlike the search endpoint, `requestLang` does NOT select a translated profile —
the response's `jvProfiles` map only ever contained the posting's own `preferredLanguage` key in
every sample pulled, so the detail response is read for its structured `locations[]` array only
(each entry: `countryCode`, `region` (NUTS), `cityName` (nullable), `postalCode` (nullable),
`addressLines` (may be empty), `buildingAddress`), never for title/description — the search
result already gives the better (translated) text for those.

**Confirmed re-listing / aggregator status.** Detail responses carry a `source` field naming the
feed the posting came from. Samples pulled this session:

| Country | `source` | Notes |
|---|---|---|
| DE | `DE001` | `applicationInstructions` link to `arbeitsagentur.de/jobsuche/jobdetail/14934-0067382855-S` — the exact reference number pattern freehire's own `arbeitsagentur` adapter already ingests directly |
| SE | `PES-SWEDEN` | matches the OpenAPI doc's own example value |
| PL | `PSZ` | Poland's public employment service; `applicationInstructions` was the bare phrase "directly to employer" — no URL at all |
| FR | `JOBTHATMAKESENS` | a private French job board, not a government PES |

This is decisive: EURES re-publishes national PES and partner-board content rather than hosting
first-party listings, the same shape `whatjobs`/`adzuna` are marked `aggregator` for.

**Application-instructions format is too inconsistent to parse.** Observed forms: a clean
`<a href="...">` (Sweden, France), an unlinked phrase with an embedded German sentence and a
separate `<a>` pointing back to the source portal (Germany), and plain prose with no URL at all
(Poland). Extracting a real apply URL would need per-language, per-source-agency prose parsing
with no reliable fallback — not attempted; `Job.URL` uses the EURES portal's own stable
`jv-details/{id}` page instead (confirmed live via web search of the current portal, since the
unofficial API docs don't document the frontend URL shape).

## Goals / Non-Goals

**Goals:**
- One provider (`eures`), one board per EURES-covered country.
- ICT-only scope enforced by the adapter's own fixed occupation filter, never board-configurable.
- Correct aggregator classification so cross-source dedup can suppress EURES copies of postings
  freehire already reaches through a more authoritative adapter (starting with the confirmed
  Germany/`arbeitsagentur` overlap, and generalizing to any other country where freehire later
  gains a direct national-PES or ATS adapter).

**Non-Goals:**
- NUTS-region sharding as a second board axis. Rejected: the measured 3-day-window headroom
  already covers today's volumes, `CompanyEntry.Board` models one identity string, and this would
  turn 31 boards into on the order of 90+ for a problem not yet observed. Revisit only if measured
  volume growth breaks the window's headroom — the first fix to try then is shortening the window
  (a one-line constant change), not adding a sharding axis.
- Parsing `applicationInstructions` for a direct apply URL (see Context above).
- Populating `boards` rows for any specific country — out of scope for this code change, done
  separately via `cmd/add-board` once the adapter is live.
- `WorkMode`/`Seniority`/`Category`/`Skills` mapping — no structured signal exists for these in
  either the search or detail response; left to the pipeline's dictionaries, as `arbeitsagentur`
  also does for the fields it has no structured signal for.

## Decisions

**Board = country, occupation filter fixed in code.** Mirrors `arbeitsagentur`'s
`berufsfeld`-as-board shape but inverted: there, the professional field IS the board and the
country is implicit (one country, Germany); here, the country is the natural board unit (EURES is
already organized by national PES feeds) and the ICT scope is the fixed, code-level filter — the
same relationship `profession.hu` has between its two dedicated IT category boards (board-selected
scope) is not available here since EURES has no "IT-only" board of its own, so the occupation
filter takes that role as a request parameter instead of a board value.

**Two-request shape (search + per-posting detail), detail used for `Location` only.** Considered
skipping the detail fetch entirely and using only the search result's `locationMap` (NUTS codes
only, no city text) — rejected because it would leave `Location` as an opaque code string like
"DE12B" for a posting even when a real city name is one request away, a worse result than most
other adapters in this catalogue produce. Considered re-deriving title/description from the detail
response too (since it is fetched anyway) — rejected because detail does not honor
`requestLang`/translation the way search does, so it would silently downgrade already-translated
English content back to the posting's original language.

**Aggregator marker, not first-party.** See Context. The alternative (treating EURES as
first-party, like `arbeitsagentur`) would let confirmed duplicate content free-ride past dedup
into the catalogue as if it were an independent posting.

**`Job.URL` is the portal page, not a scraped apply link.** See Context — the free-text
instructions are unparseable across locales; the portal's own detail page is always present and
stable.

## Risks / Trade-offs

- **[Aggregator suppression depends on the existing fuzzy/role dedup actually matching an EURES
  copy against its authoritative counterpart]** No new matching logic is introduced by this
  change — it relies entirely on the pass `AggregatorProviders` already feeds. If that pass's
  matching signal (title/company/location fuzz) is too coarse to catch a specific EURES↔ATS pair,
  an EURES duplicate will surface uncaught, same residual risk `whatjobs`/`adzuna` already carry
  today. → Not a regression this change introduces; worth another look only if EURES duplicates
  are observed in the live catalogue after rollout.
- **[Country-level recency window may not scale to every country as EURES volume grows]** See
  Non-Goals — the fix is a one-line window shrink, not urgent, not blocking this change.
- **[Detail fetch doubles the per-posting request count for a source with unmeasured rate
  limits]** Mirrors `arbeitsagentur`'s already-accepted shape (search + per-posting detail); ship
  without a pacer (the default for an unmeasured provider) and watch the first boards' crawl
  health, same posture the `gr8people` and `jobappnetwork` additions took.
