# Oil & gas job portal adapter: EnergyJobline

**Date:** 2026-10-07
**Status:** design approved, scope narrowed after a Rigzone feasibility spike

## The problem

The user supplied ~40 oil & gas domains (agencies, job portals, majors, EPC/service
companies) to evaluate as new ingest sources. Recon across all of them, plus a pass over
the live `boards` table, narrowed things down fast:

- 18 majors/EPC/service companies sit on a known ATS. 13 were already boarded
  (Shell, BP, Baker Hughes, Eni, ExxonMobil, Fluor, Halliburton, KBR, McDermott, SLB,
  TechnipFMC, Wood, Worley). 5 were added via `cmd/add-board` on 2026-10-06 (Subsea7,
  Bechtel, ADNOC, QatarEnergy, TotalEnergies). Aramco is excluded — its site TCP-blocks
  automated requests and the platform could not be confirmed.
- Of the 7 general/niche job portals (Rigzone, Oilcareers, EnergyJobline,
  OilAndGasJobSearch, Bayt, GulfTalent, NaukriGulf, Laimoon — 8 names, 7 distinct
  backends since Oilcareers shares Rigzone's infrastructure):
  - **Bayt and GulfTalent already have adapters**, already registered, and already have
    live `boards` rows. Nothing to build.
  - **Laimoon's jobs backend is dead** — confirmed stub/kill-switch across the whole
    `jobs.laimoon.com` subdomain, no link from the live homepage. Out of scope.
  - **NaukriGulf and OilAndGasJobSearch are unreachable at the network level** from both
    this environment and the prod host (tar-pit / connection refused). Out of scope.
  - **Rigzone** looked buildable on first recon (AWS WAF challenge, same shape as
    Bayt/GulfTalent's Cloudflare challenge) but a feasibility spike invalidated that — see
    below. Moved to the deferred group.
  - **EnergyJobline** is the one portal this design covers.

## Rigzone feasibility spike (2026-10-07) — INVALIDATED for now

Before committing to build Rigzone alongside EnergyJobline, its one open risk (would the
project's existing Chrome-fingerprint transport, `fingerprinthttp.go`, clear Rigzone's bot
defense the way it clears Bayt's and GulfTalent's?) was spiked directly against the live
site, using the actual transport code rather than a guess:

- **Plain fingerprint transport (`newFingerprintHTTP`, the same one `bayt.go`/
  `gulftalent.go` use):** both `https://www.rigzone.com/` and `/sitemap.xml` returned a
  flat `HTTP 403 Forbidden` (nginx-style, "a padding to disable MSIE and Chrome friendly
  error page" body) with no `x-amzn-waf-action` header at all — i.e. not the solvable
  "challenge" response seen from a plain `curl`, but a harder block that offers nothing to
  pass. This matches the pattern this repo's own `firecrawltier.go` documents for
  Bayt/GulfTalent/Wellfound: "the refusal comes before any challenge is offered" — a
  datacenter-IP-level block, not a fingerprint check.
- **Proxied fingerprint transport (`newProxiedFingerprintHTTP`, over this repo's existing
  `SOURCES_PROXY_URL`):** untestable — the proxy account returned `402 Payment Required`.
  Determining whether a non-datacenter egress clears Rigzone's block would require
  topping up that account first, a real spend decision.
- The further escalation this repo already has for exactly this shape of wall — the hosted
  Firecrawl tier (`firecrawltier.go`) — also costs money per page and was not attempted.

**Verdict: INVALIDATED within the current budget.** The free tier (fingerprint spoofing
alone) flatly fails, and the next tier that might work costs money to even test. Asked
directly, the user chose not to spend to find out — so Rigzone joins NaukriGulf and
OilAndGasJobSearch in the deferred group rather than being forced through. It can be
revisited later specifically because it is a 100%-on-topic oil & gas board (unlike Bayt/
GulfTalent, whose broad general listings make the hosted tier's cost/relevance trade-off
much worse) — if the user later wants to fund a proxy top-up or a Firecrawl trial, this
spike's numbers are the starting point, not a dead end.

## EnergyJobline

Confirmed with a plain `curl` (Googlebot UA, no cookies, no JS) — no bot defense observed:

- `https://www.energyjobline.com/sitemap.xml` is an open sitemap **index** of 4
  sub-sitemaps.
- A live posting, `https://www.energyjobline.com/job/controls-engineer-atlanta-31835232`,
  server-renders three `<script type="application/ld+json">` blocks (`WebSite`,
  `Organization`, `JobPosting`). `ldJobPosting` already walks past the non-JobPosting
  blocks (`jobPostingNode` + `isJobPosting` filter by `@type`), so no new ld+json plumbing
  is needed.
- The `JobPosting` block carries `title`, `description`, `jobLocation`/`address`,
  `hiringOrganization`, `datePosted`, `employmentType`, `baseSalary`.

This is the same enumeration shape as `dataart.go` — sitemap to enumerate, `fetchDetails`
with `defaultDetailWorkers` to fetch each page, `ldJobPosting` to decode — combined with the
same **company resolution** `bayt.go`/`gulftalent.go` already use for exactly this
situation, rather than a new mechanism:

- `boardless()` — EnergyJobline has one sitemap, no per-tenant board id.
- `aggregator()` — the existing marker interface (`internal/ingest/sources/source.go`)
  that documents "one crawl aggregates postings from many companies" and keeps the source
  included in the source facet. No `CompanyEntry.Hub`/`Tenants` involvement at all — that
  mechanism belongs to a different family of adapters (huntflow/cleverstaff/loxo/
  successfactors) whose platform does NOT hand back a clean per-posting employer name,
  so they resolve it from a URL/API field/title/curated map instead. EnergyJobline's
  `JobPosting` ld+json already carries a clean `hiringOrganization.name` per posting — the
  same situation Bayt and GulfTalent are in — so it follows their pattern exactly:

```go
company := strings.TrimSpace(p.HiringOrg.Name)
if company == "" {
    return unreadableDetail(id, link, e.Company), true // proves the posting existed; no good employer name to show
}
```

This reuses the existing `unreadableDetail` helper (`helpers.go`) rather than inventing a
fallback — the same one `bayt.go`'s `detail` uses for an empty `hiringOrganization` and for
an unparseable fetch.

## Boards

Added via `cmd/add-board` after the code ships and a first manual run looks sane:

- `energyjobline` / board `www.energyjobline.com` / company `EnergyJobline`

## Testing

A unit test over a fixture sitemap-index + 2-3 fixture job pages under different
`hiringOrganization` values, asserting: the resolved employer per posting, the
`unreadableDetail` fallback when `hiringOrganization` is absent/empty, and the dedup
`ExternalID` — the same shape as `bayt_test.go`'s existing coverage of the same pattern.

## Risks

- **Job volume is unknown.** Recon confirmed the markup shape on one posting, not a count.
  Sanity-check the first run's job count/duration before trusting the board long-term —
  the same caution the SuccessFactors hub design raised for a large, uncounted hub.
- **`hiringOrganization` has no curation step.** A scraped `hiringOrganization.name` can be
  messy — the platform's own brand, a recruiter's name instead of the real employer,
  inconsistent casing — and the fallback only catches *empty* values, not wrong-but-present
  ones. Bayt and GulfTalent accept the same risk for the same reason: it is the only
  per-posting signal the site offers. Watch the catalogue after launch for obviously wrong
  employer names on this source specifically.

## Out of scope

- **Rigzone (and Oilcareers)** — spiked and invalidated within the current budget (see
  above). Revisit if the user wants to fund a `SOURCES_PROXY_URL` top-up or a Firecrawl
  trial specifically for it.
- Laimoon — jobs backend is confirmed dead, not resurrectable by an adapter.
- NaukriGulf, OilAndGasJobSearch — unreachable at the network level from this environment
  and from the prod host; the same paid-capability question as Rigzone would apply, not
  attempted.
- Any change to `bayt.go`/`gulftalent.go` or the `aggregator`/`boardless` marker
  interfaces — EnergyJobline only consumes them, exactly as they already exist.
