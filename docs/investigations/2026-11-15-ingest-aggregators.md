# Aggregator ingest investigation, 2026-11-15

Read-only production investigation from operator (89.167.94.146); health read from core's
`hire.board_health`. No crawl, deployment, cooldown reset or database mutation was performed.
Probes request individual public URLs, not entire catalogues. Secret values and token responses
were never printed. HTTP routes below use the existing Scrapling service; “proxy” uses its
configured egress, not a new credential. Results describe this observation window, not a
promise that a sustained crawl will pass the same edge.

## The Muse

- Initial: one failing board, 10 consecutive failures; last success November 8.
- Error: `/api/public/jobs?page=1` returns 404.
- Live: plain operator curl gets Cloudflare 403; Scrapling HTTP reaches the origin and gets
  404 with The Muse's HTML shell. Page 0 with an empty `api_key` also gets 404.
  `/developers/api/v2` redirects to `/about` (200); `api.themuse.com` cannot connect.
- Confirmed: the historical public API route no longer serves JSON; changing the initial
  page or fingerprint does not restore it. A `/search` probe is also challenged.
- Fix/blocker: no verified replacement feed. Do not swallow 404 or return an empty catalogue.
  Obtain a supported feed/API from The Muse before changing this adapter. A new HTML crawler
  would be a separately validated replacement, not an endpoint spelling fix.

## EchoJobs

- Initial: one failing board, 21 failures; last success October 25.
- Error: sitemap index HTTP 403.
- Live: the configured browser + proxy route returns an explicit `403: Forbidden` HTML page
  (~30 KB). Scrapling HTTP direct and browser direct return the same refusal.
- Confirmed: listing access is refused before parsing, including the formerly working route.
- Blocker: provider access/egress approval or a newly measured working route. No adapter fix
  can parse this refusal into jobs; preserve the health failure.

## France Travail

- Initial: all 101 départements failing, up to 14 failures each.
- Error: OAuth token endpoint returns 400.
- Live: one token request with the deployed application credentials and the adapter's exact
  scopes returns 400, `invalid_client`, `Client authentication failed`.
- Confirmed: authentication fails before the search endpoint. This repeats the November 11
  finding already recorded in the provider note; it does not distinguish revoked secret,
  disabled application, or subscription loss.
- Blocker: owner must inspect the application's developer-portal status/subscription and
  restore credentials. Verify token issuance and one authenticated search before an approved
  retry; do not alter scopes or reset health to conceal it.

## Gradcracker

- Initial: one failing board, 13 failures.
- Error: first computing-technology listing returns 403 on the deployed fingerprint route.
- Live: Scrapling HTTP direct returns 200, 1,102,416 characters, the real Computing,
  Technology & IT listing, redirected to `/search/technology-it/jobs?order=dateAdded`.
- Confirmed: the listing remains available through another fingerprint; the old path redirects
  rather than disappearing. This is transport access, not an empty discipline.
- Production follow-up for parent: rebuild this provider over Scrapling HTTP direct while
  preserving `pacedGradcracker`, its ten-second request interval and hydration budget. This
  changes shared `scraplingtier.go`, outside this batch's ownership. Validate another listing
  and one posting with the parser, then a bounded approved crawl; one successful probe does
  not establish that the hourly IP budget has disappeared.

## InfoJobs Brasil

- Initial: all 28 boards failing, up to 23 failures.
- Error: state listings return 403.
- Live: Scrapling HTTP direct returns 403, title `Azure WAF`; the identical Acre URL through
  Scrapling HTTP + proxy returns 200 and a real 232,078-character listing.
- Confirmed: Azure WAF refuses direct egress; a measured proxy route serves the listing.
- Follow-up for parent: route through Scrapling HTTP + proxy in the shared tier, retaining
  the session cookie jar, shared pacing, board budgets and 4500-second scheduler timeout.
  Verify fragment pagination with the `ab_session_id` cookie and one detail before deploying;
  a transport that loses that cookie would repeat page 1. No production change made here.

## 104 Job Bank

- Initial: all 344 area boards failing, up to 15 failures.
- Error: search API first page returns 403.
- Live: Scrapling HTTP + rotating proxy (configured route), HTTP direct, and browser + proxy
  all return 403 and 104's own Chinese error page (~166 KB), not search JSON. The page says
  the site is broken and to wait/contact support. Tested area 6001023000, page 1, with a
  same-origin Referer. A repeat with the adapter’s exact
  `https://www.104.com.tw/jobs/search/` Referer also returns the same 403.
- Confirmed: the configured search endpoint is refused across those lanes. The response
  does not establish whether an endpoint migration or access policy caused the refusal.
- Blocker: no working search API/egress combination demonstrated. Do not remove the error,
  alter area splitting, or launch 344 full crawls against it. Next investigation needs a
  successful browser search request trace to establish the site's current search protocol.

## Mynavi

- Initial: both occupation boards failing (6/5 failures); last successes November 13.
- Actual journal error: every attempted detail says `page carries no JobPosting`; listing
  access succeeds, so the final “none answered” error is parser failure, not a proxy outage.
- Live: proxied `/jobinfo-328792-2-29-1/` returns 200 and a real 173,228-character internal-SE
  advert. Breadcrumb JSON-LD remains; JobPosting JSON-LD is absent. The visible description,
  candidate requirements and employment/benefit table are intact. A second live posting,
  437778-5-93-1, likewise returns 200 (187,735 characters) without JobPosting JSON-LD.
- Fix: fall back to those visible advert sections only when JobPosting cannot be read.
  Require `og:url` to match the requested posting exactly, retain listing identity/facts,
  and keep JSON-LD as the preferred path. Do not guess structured salary or place fields.
- Verification: recorded live HTML sections reproduce `page carries no JobPosting` before
  the change. The regression then reads duties, requirements, location and benefits and
  rejects a different posting identity. Existing JSON-LD tests remain in the targeted run.
- Follow-up: deploy through the normal parent release, then observe the next due run and
  hydrated bodies. No cooldown reset is needed for correctness.

## Optius

- Initial: one failing board, 4 failures; last success November 14.
- Actual journal error: the same three remaining postings (965951, 965950, 965718) return
  `posting page carries no body` despite successful listing walks.
- Live: 965950 remains in the sitemap; its detail returns 200, an image-only visible advert,
  and substantial JobPosting text. `responsibilities` contains raw newlines AND unescaped
  quotation marks around `okužiš`. Shared JSON-LD handling repairs control characters but
  strict JSON decoding rejects the raw quotes, losing the only machine-readable body. A
  second failing posting (965718) also returns 200 with invalid JobPosting JSON after
  newline normalization.
- Fix: an Optius-only fallback escapes unescaped quotation marks within the three text body
  fields, bounded by the next property. Accept the repair only if the entire JobPosting
  decodes. Keep the normal shared parser first; no shared parser/transport was changed.
- Verification: the recorded live JobPosting plus image-only section fails the old detail
  path with `posting page carries no body`; the regression requires real description,
  responsibilities, employer introduction, employment type and deadline after repair.
- Follow-up: normal deployment and next due run; keep genuine missing-body pages failing.

## SpainJobs

- Initial: one failing board, 17 failures; last success October 29.
- Error: sitemap index returns 403.
- Live: Scrapling HTTP direct returns Cloudflare `Attention Required!` (403); HTTP + proxy
  returns the actual index (200) and its `/sitemap/jobs-new.xml` shard (200, 438,022 characters).
  The adapter already selects `/sitemap/jobs-`; there is no sitemap-path bug in this checkout.
- Confirmed: direct egress refusal; a proxy route serves both levels of listing XML.
- Follow-up for parent: shared tier entry using Scrapling HTTP + proxy, preserving adapter
  pacing and hydration limits. Verify one posting and its description before deployment.

## JobStreet / JobsDB

- Initial: 45 of 132 market/subclassification boards failing, up to 20 failures; 87 have
  healthy state, so this is not a network-wide API removal.
- Error sampled: Thailand subclass 6303 first-page search returns 403.
- Live: the exact TH-Main/Thailand/subclassification=6303 request returns Cloudflare
  `Attention Required!` under direct HTTP, Scrapling HTTP + proxy, and browser direct.
- Hong Kong also returns 403 through Scrapling HTTP + proxy. Health groups show 22/22 HK
  and 22/22 TH failing; MY, ID and SG have no current failures. The remaining PH failure
  is transient: its exact subclass 6296 query now returns 200 and JSON (142,086 characters).
- Confirmed: this market's listing endpoint is refused before JSON parsing; the existing
  GraphQL detail fallback cannot help a listing request that never discovers posting ids.
- Blocker: no working Thailand listing route demonstrated. Parent owns the shared SEEK
  network; do not rewrite or reclassify the healthy markets, suppress failures, or change
  detail pacing to solve a listing refusal. Measure a working route per affected market.

## Validation and release boundary

The changes are limited to Mynavi/Optius adapter details, recorded fixtures, tests and notes.
The parent must coordinate shared transport routing for the three measured recovery candidates.
No production data, scheduling, cooldowns, services or host configuration was changed.

Validation completed locally:

- `gofmt -l .`: no output.
- `go test ./internal/ingest/sources -run 'Optius|Mynavi' -count=1`: passed.
- `go vet ./...`: passed.
- `go test ./...`: passed.
- `go vet -tags=integration ./...`: passed.
- `git diff --cached --check`: passed.
- Simplify pass: retained the two provider-local helpers and existing shared APIs.
- CodeRabbit 0.9.5: first completed review found two valid Optius boundary cases (adjacent
  body fields and a final body field); both fixed with regressions. Second completed review
  covered all ten adapter/test/note/fixture files and reported zero findings.
