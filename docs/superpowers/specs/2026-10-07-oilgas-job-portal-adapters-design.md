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

This is the same shape as `dataart.go`: `resolveSubSitemap` (or direct `sitemapJobLocs`
per sub-sitemap, since the index is small) to enumerate job URLs, `fetchDetails` with
`defaultDetailWorkers` to fetch each page, `ldJobPosting` to decode it.

**It is a hub, not a single-company board.** EnergyJobline carries postings from many real
employers across energy broadly (oil & gas is one of its categories). The adapter must
resolve `Company` per posting from the ld+json `hiringOrganization.name`, not from the
board's configured company name.

## New pattern: hub resolution from `hiringOrganization`

All four existing hub adapters (`huntflow`, `cleverstaff`, `loxo`, `successfactors`)
resolve the employer from something OTHER than the JobPosting ld+json's own
`hiringOrganization` field — a URL segment, an API field, a title, or a curated `Tenants`
map. EnergyJobline offers none of those signals; the only per-posting employer signal is
`hiringOrganization` itself. This design adds that as a new (but small, additive)
resolution path:

```go
// hiringOrg is the schema.org Organization embedded in a JobPosting's hiringOrganization
// field. For a hub adapter with no better per-posting signal, its Name is the posting's
// real employer; the caller falls back to ce.Company when Name is empty, matching the
// existing hub adapters' fallback philosophy (an unrecognised/missing signal keeps the
// job visible under the hub's own name rather than guessing).
type hiringOrg struct {
    Name string `json:"name"`
}
```

The posting struct embeds a `HiringOrganization hiringOrg
\`json:"hiringOrganization"\`` field; `detail` sets `Company` from it when `ce.Hub` is
set and the name is non-empty, else `ce.Company`.

This does not change `CompanyEntry`, `Hub`, or any existing adapter — it is a second way to
honour the already-generic `Hub` flag, parallel to the three existing ones. With only this
one call site, no shared helper is extracted yet (see Out of scope).

## Boards

Added via `cmd/add-board` after the code ships and a first manual run looks sane:

- `energyjobline` / board `www.energyjobline.com` / company `EnergyJobline` / `--hub`

## Testing

A unit test over a fixture sitemap-index + 2-3 fixture job pages under different
`hiringOrganization` values, asserting: the resolved employer per posting, the
`ce.Company` fallback when `hiringOrganization` is absent/empty, and that a non-hub
`CompanyEntry` (hypothetical) still takes `ce.Company` unchanged — the same shape as
`successfactors_hub`'s existing test.

## Risks

- **Job volume is unknown.** Recon confirmed the markup shape on one posting, not a count.
  Sanity-check the first run's job count/duration before trusting the board long-term —
  the same caution the SuccessFactors hub design raised for a large, uncounted hub.
- **`hiringOrganization` has no curation step.** Unlike SuccessFactors' `Tenants` map
  (hand-verified once), a scraped `hiringOrganization.name` can be messy — the platform's
  own brand, a recruiter's name instead of the real employer, inconsistent casing — and
  the fallback only catches *empty* values, not wrong-but-present ones. This is accepted
  because it is the only per-posting signal the site offers; watch the catalogue after
  launch for obviously wrong employer names on this hub specifically.

## Out of scope

- **Rigzone (and Oilcareers)** — spiked and invalidated within the current budget (see
  above). Revisit if the user wants to fund a `SOURCES_PROXY_URL` top-up or a Firecrawl
  trial specifically for it.
- Laimoon — jobs backend is confirmed dead, not resurrectable by an adapter.
- NaukriGulf, OilAndGasJobSearch — unreachable at the network level from this environment
  and from the prod host; the same paid-capability question as Rigzone would apply, not
  attempted.
- Any change to `Hub`/`Tenants` semantics for the four existing hub adapters.
- A generic "resolve hub from `hiringOrganization`" helper shared across adapters — with
  only one call site, a shared helper is premature; revisit if a second hub needs the same
  shape.
