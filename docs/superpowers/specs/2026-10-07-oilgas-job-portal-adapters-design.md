# Oil & gas job portal adapters: EnergyJobline + Rigzone

**Date:** 2026-10-07
**Status:** design approved, pending written-spec review

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
  - **Bayt and GulfTalent already have adapters** (`internal/ingest/sources/bayt.go`,
    `gulftalent.go`), already registered in `registry.go` behind the fingerprint-spoofed
    transport, and already have live `boards` rows (8 Bayt country boards + 1 GulfTalent,
    all `status=active`). Nothing to build.
  - **Laimoon's jobs backend is dead.** `jobs.laimoon.com` serves a CloudFront-Function
    stub titled "Laimoon Jobs - Coming Soon" on every path, including the bare root;
    `www.laimoon.com/sitemap.xml` redirects to `courses.laimoon.com/sitemap.xml` (the
    courses product's sitemap, not jobs); `courses.laimoon.com/jobs` returns a permanent
    "High traffic" stub; the live `www.laimoon.com/` homepage carries no link to the jobs
    subdomain at all. Google still has stale job URLs indexed, but every one of them now
    404s. Out of scope — there is no backend left to adapt to.
  - **NaukriGulf and OilAndGasJobSearch are unreachable at the network level**, not just
    bot-detected: `naukrigulf.com` completes a TLS handshake but never returns an HTTP
    response (tar-pit behaviour), and `oilandgasjobsearch.com` refuses the TCP connection
    outright. Both failures reproduce identically from this environment and from the prod
    host (`89.167.94.146`), so this is not an IP-reputation problem a stealth transport
    fixes — it would need a residential proxy or a headless-browser service, a separate
    paid capability and decision. Out of scope for this design.
  - **EnergyJobline and Rigzone (covers Oilcareers)** are the two portals this design
    covers.

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

## Rigzone (and Oilcareers)

Both `www.rigzone.com` and `www.oilcareers.com` currently return an AWS WAF challenge to a
plain client (`HTTP 202`, header `x-amzn-waf-action: challenge`) — the same shape
`registry.go` already documents for `bayt`/`gulftalent`/`jobleads` ("the plain Go client
simply 403s on the challenge response"). `oilcareers.com`'s TLS certificate does not cover
its own hostname (it appears to be served off Rigzone's certificate/backend), consistent
with the two domains having merged in 2014. The adapter is named and boarded as
`rigzone`, targeting `www.rigzone.com` only — `oilcareers.com` is not separately crawled;
doing so would just duplicate the same postings under a second URL.

Build it the same way as `bayt.go`/`gulftalent.go`: construct it with the fingerprint-spoofed
transport (`fp`) already wired into `registry.go`'s special-transport branch, not the plain
client. Everything past the transport — sitemap discovery, per-job ld+json decode, hub
resolution from `hiringOrganization` — follows the same template as EnergyJobline.

The exact sitemap path and job URL shape could not be confirmed directly: the WAF blocks
the fetch from both this environment and the prod host's current egress path, so there was
no way to inspect the site's HTML/XML independent of the `fp` transport. **The first step
of implementation is confirming `fp` actually clears the challenge** and, once it does,
confirming the sitemap and ld+json shape against the live site — before writing any
parsing struct. If `fp` does not clear Rigzone's WAF, this adapter is blocked the same way
NaukriGulf/OilAndGasJobSearch are, and should move to the deferred group rather than be
forced through.

## Shared new pattern: hub resolution from `hiringOrganization`

All four existing hub adapters (`huntflow`, `cleverstaff`, `loxo`, `successfactors`)
resolve the employer from something OTHER than the JobPosting ld+json's own
`hiringOrganization` field — a URL segment, an API field, a title, or a curated `Tenants`
map. Neither EnergyJobline nor Rigzone offers any of those signals; the only per-posting
employer signal on either site is `hiringOrganization` itself. This design adds that as a
new (but small, additive) resolution path:

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

Each adapter's posting struct embeds a `HiringOrganization hiringOrg
\`json:"hiringOrganization"\`` field; `detail` sets `Company` from it when `ce.Hub` is
set and the name is non-empty, else `ce.Company`.

This does not change `CompanyEntry`, `Hub`, or any existing adapter — it is a second way to
honour the already-generic `Hub` flag, parallel to the three existing ones.

## Boards

Added via `cmd/add-board` after the code ships and a first manual run looks sane:

- `energyjobline` / board `www.energyjobline.com` / company `EnergyJobline` / `--hub`
- `rigzone` / board `www.rigzone.com` / company `Rigzone` / `--hub`

## Testing

- Per adapter: a unit test over a fixture sitemap (or sitemap-index, for EnergyJobline) +
  2-3 fixture job pages under different `hiringOrganization` values, asserting: the
  resolved employer per posting, the `ce.Company` fallback when `hiringOrganization` is
  absent/empty, and that a non-hub `CompanyEntry` (hypothetical) still takes `ce.Company`
  unchanged — the same shape as `successfactors_hub`'s existing test.
- Rigzone additionally needs a one-off manual check (not necessarily a committed test)
  that the `fp` transport clears the live WAF challenge, since that cannot be verified
  from a fixture.

## Risks

- **Rigzone's WAF may not accept `fp`.** Bayt/GulfTalent/Jobleads needed the fingerprint
  transport to clear their *own* challenge; there is no guarantee Rigzone's AWS WAF
  configuration accepts the same spoofed fingerprint. See the open question above.
- **EnergyJobline's job volume is unknown.** Recon confirmed the markup shape on one
  posting, not a count. Sanity-check the first run's job count/duration before trusting
  the board long-term — the same caution the SuccessFactors hub design raised for a
  large, uncounted hub.
- **`hiringOrganization` has no curation step.** Unlike SuccessFactors' `Tenants` map
  (hand-verified once), a scraped `hiringOrganization.name` can be messy — the platform's
  own brand, a recruiter's name instead of the real employer, inconsistent casing — and
  the fallback only catches *empty* values, not wrong-but-present ones. This is accepted
  because it is the only per-posting signal either site offers; watch the catalogue after
  launch for obviously wrong employer names on these two hubs specifically.

## Out of scope

- Laimoon — jobs backend is confirmed dead, not resurrectable by an adapter.
- NaukriGulf, OilAndGasJobSearch — unreachable at the network level from this environment
  and from the prod host; would need a paid residential-proxy or headless-browser
  capability, a separate decision with its own cost.
- Any change to `Hub`/`Tenants` semantics for the four existing hub adapters.
- A generic "resolve hub from `hiringOrganization`" helper shared across adapters — with
  only two call sites so far, a shared helper is premature; revisit if a third hub needs
  the same shape.
