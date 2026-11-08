## Context

`internal/collections` already generalizes company tags into three kinds
(`KindEditorial`, `KindBacker`, `KindCredential`) with one registry (`All` in
`collections.go`), a common member-resolution path in `cmd/import-collections`, and
a shared conservative-matching toolkit in `register.go` (`RegisterSlug`,
`RequireCountry`, `DropAmbiguous`). Two credentials exist today — GOV.UK's Skilled
Worker sponsor register and IND's recognised-sponsor register — each contributed as
one registry entry plus one source file (`uksponsor.go`, `nlsponsor.go`). This
design adds a third: `us-h1b-sponsor`, sourced from USCIS's public H-1B Employer
Data Hub.

The relevant part of the source was verified live during design (not assumed):

- Index page `https://www.uscis.gov/archive/h-1b-employer-data-hub-files` lists one
  link per completed fiscal year with stable markup:
  `<a href="/sites/default/files/document/data/h1b_datahubexport-{YEAR}.csv" ...>FY {YEAR} H-1B Employer Data</a>`.
  At verification time (2026-08-04) the newest listed year was FY2023.
  Notably: `curl` with a browser User-Agent got HTTP 200; the design-time fetch
  tool that omits one got HTTP 403 — the site's bot filter keys on client
  fingerprint, not on "any automated request."
- Each year's CSV (verified against a downloaded FY2023 sample, ~33k rows) has
  header `"Fiscal Year",Employer,"Initial Approval","Initial Denial","Continuing
  Approval","Continuing Denial",NAICS,"Tax ID",State,City,ZIP`. `Tax ID` is
  USCIS's documented last-4-digits-of-TIN field — an per-organisation identity
  fragment, not a full EIN, but sufficient as the ambiguity guard's identity key
  (the same role `town` plays for the UK register and `kvk` for the Dutch one).
  A given employer's `Tax ID` repeats identically across its rows for different
  years and worksite locations within a year.

## Goals / Non-Goals

**Goals:**

- Add `us-h1b-sponsor` as a `KindCredential` entry using the existing dataset/gate
  machinery unchanged.
- Source it from the USCIS Employer Data Hub bulk files, combining the most recent
  5 completed fiscal years into one member list per run.
- Grant the credential to an employer with at least one approved petition (initial
  or continuing) across that 5-year window; treat a company with only denials in
  the window as not sponsoring.
- Extend the shared matching toolkit (`register.go`) with what US employer names
  need: country aliases and corporate-suffix stripping.

**Non-Goals:**

- DOL/OFLC LCA disclosure data (job title, wage, worksite per petition). That
  dataset answers a different question — "what does a specific role pay" — and is
  a candidate for a future job-level feature, not a company credential fact.
- A route/tier gate. GOV.UK's register lists Temporary Worker and study routes
  alongside Skilled Worker in the same file, which is why that credential needs a
  route gate; the Data Hub is H-1B-specific from the source, so no equivalent
  filtering is needed.
- Surfacing approval counts, denial rates, or which fiscal years matched in the UI.
  The badge is presence/absence only, exactly like the UK and NL badges.
- Any change to `cmd/import-collections`, the ambiguity guard, or the
  `/collections/[slug]` routes — all three are generic over `Kind`/`Dataset`
  already and need no US-specific branch.

## Decisions

**Data source: USCIS Employer Data Hub, not DOL/OFLC LCA disclosure data.**
The Data Hub is pre-aggregated per employer per fiscal year with exactly the shape
a credential needs (approved vs. denied counts); LCA disclosure files are
per-application, 75+ columns, and quarterly — richer, but at the grain of a
different feature. Matching the Hub's grain to the existing credential shape keeps
this change additive rather than introducing a new kind of data model.

**Self-fetching `Dataset.Records`, not `Dataset.URL`/`Parse`.**
UK and NL each resolve to *one* URL per run. This register must combine 5 separate
year-files into one member list, which the `Dataset.URL`/`Parse` split cannot
express (`Parse` receives one already-fetched body). `speedrun.go`'s
`fetchSpeedrunDirectory` already establishes this precedent — a `Dataset.Records`
function that owns its own fetch-and-combine loop end to end. The same "read
completely or fail" contract documented on `Dataset.Records` applies: if any of the
5 year-fetches fails, the whole call fails rather than returning a 4-year result,
because a silently shrunk source is indistinguishable from a real drop in
sponsorship and would read as one.

**A fixed window of 5 fiscal years, discovered dynamically, not pinned by year
number.**
Pinning literal years (e.g. `2019`–`2023`) would silently go stale as USCIS
publishes new years and stops meaning "last 5" within one annual cycle. Instead,
`discoverRecentFYFiles` parses every (year, URL) pair the index page currently
lists, sorts descending, and takes the top 5 — the same "resolve at fetch time"
principle the UK register already uses for its dated snapshot URL, adapted from
"resolve 1" to "resolve top-N."

**Approval threshold enforced during parsing, not via a `Gate`.**
`InitialApproval + ContinuingApproval < 1` rows are dropped while building the
member list, before matching. A `Gate` was considered (mirroring `RequireRoute`),
but the threshold is a fact about the *source row*, not about the *matched
company* — `RequireRoute` exists as a gate because the UK register's route is read
per-row after a name match, to decide whether that specific row counts; here,
"has any approval" is decided once per row, independent of which company (if any)
the row eventually matches, so filtering at parse time is simpler and keeps the
Gate slot free for what it is for (`RequireCountry("US")`).

**Identity key: `Tax ID` (last-4-digits), not city/state.**
Unlike the UK register (`town`) or a company/worksite pairing, `Tax ID` is a
fragment of the employer's actual legal identifier rather than a geography field,
so it disambiguates same-named organisations more reliably than a worksite city
would (a single employer legitimately files from many worksite cities in one
year). The Hub's own documentation states this field exists precisely to
distinguish employers.

**`legalSuffixes` gains US corporate forms but deliberately excludes `Co`.**
`Inc`, `Incorporated`, `Corp`, `Corporation`, `LLC` are added following the
existing map's own stated policy — narrow and evidence-based, only forms this
specific register actually emits, because a speculative addition widens the
over-strip blast radius for every register, not just this one. `Co` is excluded
because, unlike the others, it collides with ordinary two-letter words and
abbreviations inside genuine company names (not just as a trailing legal-form
token) — a judgment call, revisit if the US register's own false-negative rate
(measured before deploy, per the existing convention) shows it matters.

## Risks / Trade-offs

- **[Risk — materialized] uscis.gov 403s the production Hetzner datacenter IP.**
  Confirmed on 2026-08-05: the first real `cmd/import-collections` run against
  production failed with a 403. A User-Agent fix (matching the design-time
  curl-vs-tool discrepancy noted above) was insufficient — `curl` run directly
  *on* host-2 with a browser User-Agent still got 403, proving this is IP
  reputation against the Hetzner range, not a client-fingerprint check. Fixed by
  routing the `us-h1b-sponsor` fetch through the existing `SOURCES_PROXY_URL`
  residential-proxy mechanism (`internal/sources/proxy.go` already solves this
  exact class of problem for several board providers — reused rather than
  reinvented), opt-in by collection slug so the UK/NL registers, which have never
  needed it, stay on the direct IP.
- **[Risk] The Hub's bulk-file archive lags the live USCIS query tool by roughly
  2-3 fiscal years** (archive topped out at FY2023 at verification time; the query
  tool covers into FY2026). → Mitigation: accepted trade-off — the alternative is
  scraping a JS-driven query UI for a badge whose whole point is "has this
  employer sponsored H-1B historically," a signal that tolerates a few years of
  lag far better than, say, a real-time filter would.
- **[Risk] Last-4-digit TIN is a weak identifier in isolation** (10,000 possible
  values). → Mitigation: `DropAmbiguous` only compares identities *within* rows
  sharing the same normalized name — a coincidental Tax ID collision only matters
  if two organisations also share the same normalized name, a materially rarer
  compound coincidence, and the existing ambiguity guard already accepts this
  trade-off for the UK/NL identity fields.
- **[Risk] Measured false-positive rate is unknown before this design is
  implemented** (the UK/NL rollout measured 457/7,944 and 83/2,732 with no false
  positive in the top 40 before deploy). → Mitigation: tasks.md includes a
  measurement step against the live US company set before this ships, following
  the same convention.

## Migration Plan

No schema migration: `collections` is already a filterable column/attribute on
both the `companies` and `jobs` Meilisearch indexes. Deploying this change adds a
new possible value to that existing attribute; the next scheduled
`cmd/import-collections` run picks it up and the next `make reindex` (never
stacked with `reindex-companies`) makes it filterable. Rollback is deleting the
registry entry and re-running the import worker, which clears the tag from every
company that held only this credential — no destructive migration either
direction.

## Open Questions

- None outstanding — scope, source, window, threshold, and UI presentation were
  all settled with the user before this design was written.
