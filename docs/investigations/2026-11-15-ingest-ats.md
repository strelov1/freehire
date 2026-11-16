# ATS ingest investigation, 2026-11-15

Production investigation was read-only through operator and core. No workers were started,
no catalog or health rows were changed, and no cooldowns were reset. Counts below are the
initial `board_health` snapshot, not a count of active catalog rows: health can outlive its
catalog entry. `consecutive_failures > 0` also includes boards whose cooldown has expired.
The status page's cooled count is `cooldown_until > now()`, so these figures differ.

## JOIN

- Initial state: 17,321 health identities, 9,348 currently cooled. 15,089 had nonzero
  failure counters. Many errors were listing HTTP 404; sample company 108448 had 15 failures.
- Reproduction: `GET https://join.com/api/public/companies/54/jobs?page=1&pageSize=5`
  returned 404 `Cannot GET /api/public/companies/54/jobs` from laptop and operator.
  The old detail API `/api/public/jobs/14771122` also returned 404.
- Live valid control: `https://join.com/companies/join` still serves JOIN (company 54)
  and three job listings. Its current public JS uses
  `https://join.com/candidate-api/graphql`, `PublicJobsQueryInput` and `PublicJobInput`.
  POST `publicJobs(input:{companyId:54,paginationPage:{page:1,pageSize:5}})`
  returned three identities and complete `pageInfo`; `publicJob(input:{id:14771122})`
  returned `descriptionHtml` and `company.domain`.
- Cause: removed REST endpoints, not evidence that all catalog companies were deleted.
- Fix: use the site's GraphQL queries for listing and detail; retain native external IDs,
  display URLs, rate limiting, pagination checks, and unreadable detail protection.
  Fail HTTP-200 GraphQL errors even if partial data is present.
- Empty evidence changed: company 176781 and unknown 999999999 both return an explicit
  zero envelope. Remove `emptyBoardListing`; this API cannot establish verified emptiness
  for a company identity. Do not retire company 108448 merely because either API is empty.
- Verification: targeted tests cover mapping, pagination, missing/partial data, repeated
  pages, changing totals and unreadable details. A temporary read-only live adapter probe
  fetched all three JOIN jobs with nonempty descriptions. No production writes.
- Integration: the one JOIN registry expression and obsolete JOIN-only pacing helper
  change with the adapter; shared transport behavior is untouched. The shared empty-marker
  table drops JOIN's row because the adapter no longer grants that capability.

## Intervieweb

- Initial state: 5 health identities, 1 cooled. MSC Technology Italia had 8 failures:
  `listed 1 new postings and read none of their details`; last success November 11.
- Live probe: ordinary GET `/en/career` returns 200, 15 links and `Page 1 of 2`.
  `/jobs/agile-coach-118174/en/` returns 200 with valid JobPosting JSON-LD, title
  `Agile Project Manager`, description and dates. These are live valid company pages.
- A probe with `X-Requested-With: XMLHttpRequest` returns only a shell; this is not the
  adapter's request and is not evidence of missing listings. Normal GET must be the control.
- Existing database rows include readable descriptions for all first-page identities.
  The historical failure concerns a single newly hydrated identity, not a platform-wide
  listing outage. Do not retire MSC or relax the all-details-unreadable guard without
  identifying that identity's response. Additional incremental probe result recorded below.

## Mindsight

- Initial state: 2 health identities, 1 cooled. `alsglobal` had 14 failures:
  `stopped after 10 redirects`, with no prior success recorded.
- Live probe: both operator and laptop reproduce a 307 redirect loop for `/alsglobal`.
  Active catalog control `/grupomngt` also loops on the laptop. Increasing the redirect
  bound is not a fix: curl follows 50 redirects and still fails.
- Cause/blocker: upstream careers routing loop. No evidence justifies retiring ALS;
  another company's route fails too. No parser change. Recheck a bounded GET after the
  provider fixes routing, then allow the ordinary scheduled retry to self-heal.

## OpenCATS

- Initial state: 10 health identities, 2 cooled.
- `dwrs.mggroup.lk/careers/index.php?m=careers&p=showAll`: 39 failures, last success
  October 7; both operator and laptop return 503 (162-byte response).
- `itsource.indovisionglobal.com/careers/index.php?m=careers&p=showAll`: 23 failures,
  last success October 25; operator returns 500 (723 bytes), laptop DNS resolution fails.
- Cause/blocker: these self-hosted installations fail before a listing can be parsed.
  A provider-wide endpoint substitution would be wrong: these are independent hosts.
  Neither response proves permanent company deletion. Keep health failures visible and
  recheck after the employers repair hosting/DNS. No code or catalog change.

## PageUp

- Initial state: 204 health identities, 42 cooled. Nonzero health errors comprise
  26 WAF challenges, 9 HTML-as-JSON decode errors, and 7 HTTP 404s.
- Live probes from operator and laptop: institution 893 redirects to
  `https://careers.madergroup.com.au/jobs/search` (200 HTML, 222,529 bytes);
  institution 784 redirects to
  `https://armaguard-careers.careerpages.rec-marketing.dc2.pageuppeople.com/jobs/search`
  (200 HTML, 147,947 bytes). Both remain live employers. Institution 889 still returns
  the expected old XHR JSON envelope (200, 393,263 bytes).
- Cause: at least two cooled boards migrated careers products. Current PageUp's canonical
  `/<id>/cw/en/search/` API remains valid for other tenants; replacing its endpoint globally
  would break those tenants. The redirects' destination is a different HTML career product,
  so a JSON decoder fix or a WAF transport override alone cannot recover these two.
- Follow-up: validate the existing Clinch adapter against these destination hosts, then
  propose explicit catalog moves preserving company identity. This is catalog ownership
  work for the parent; do not retire live companies or reset cooldowns. The remaining
  challenge/404 identities need per-board bounded probes before any retirement; no blanket
  retirement is justified by this sample. Shared WAF transport changes are outside this batch.

## PeopleSoft

- Initial state: 6 health identities, 2 cooled. Winnipeg and District of Columbia each
  had 5 failures: `listed 1 new postings and read none of their pages`; both last succeeded
  November 13. This concerns one new opening per board, not every job detail.
- Live controls: DC with a cookie jar serves its search page (246,984 bytes) and opening
  33995 under `PostingSeq=1` (53,216 bytes), with matching
  `HRS_SCH_WRK2_HRS_JOB_OPENING_ID`. Winnipeg's cookie-backed listing serves 229,095
  bytes and identifies openings 127551, 127549 and 127553.
- A jar-less DC curl redirects to a cookies-required login screen. The adapter already
  uses a cookie session; that curl is not a reproduction of its detail failure.
- Incremental probes identified the exact failures: DC 33966 and Winnipeg 125461.
  Both are listed and serve valid matching details under `PostingSeq=2`; sequence 1
  returns a search page. Omitting the sequence also returns a search page. Sequence 3
  does not serve either opening.
- Cause: adapter hardcoded the original posting sequence even for reposted openings.
- Fix: after a recognized search-page response to sequence 1, try sequence 2 and require
  the actual opening ID to match. Store the successful sequence URL. Do not retry transport
  failures or accept a different opening. Regression covers the search-to-detail transition.
  This bounded compatibility fallback does not discover arbitrary future posting sequences.
  Neither live board should be retired.

## Senior

- Initial state: 83 health identities, 59 cooled. 77 had nonzero failure counters:
  76 HTTP 403 responses and one older 404 for `pxcenter` (48 failures, last success July 21).
- Live probes: POST the existing `getProfileIdBySubdomain` endpoint with JSON for known
  tenant `intelbras` and old failing tenant `pxcenter`: both return 403 with an empty body,
  on both operator and laptop. Recent valid tenants such as `atriohoteis` succeeded November 12.
- Cause/blocker: upstream anonymous endpoint access is refused before tenant resolution.
  The old `pxcenter` 404 cannot be interpreted while a valid control is refused as well.
  Do not convert current 403s to gone or retire the tenant set. A measured alternate
  transport/access route is needed; shared transport ownership belongs to the parent.

## TalentBrew

- Initial state: 9 health identities, 1 cooled. SC Johnson had 14 failures and no recorded
  success: `listed 130 new postings and read none of their details`.
- Live probe: `https://jobs.scjohnson.com/en/job/racine/manager-media-and-integrated-marketing/5362/102078701904`
  returns 200 with the actual title, location and complete `.ats-description`, but zero
  JobPosting JSON-LD blocks. The listing and employer remain valid.
- Cause: adapter required JSON-LD even though this theme publishes its job as identified HTML.
- Fix: fall back to `section[data-selector-name=jobdetails][data-job-id=<listed id>]`;
  read its h1, `.ats-description` and `.ajd_header__location`, sanitize the body, and
  refuse missing bodies or mismatched identities. Do not invent an absent posted date.
- Verification: regression covers successful HTML mapping and rejection of a wrong posting
  identity; temporary probe parsed the captured real SC Johnson page successfully.

## Tenstreet

- Initial state: 465 health identities, 64 cooled; all 64 failure messages say that new
  postings were listed but none of their detail pages could be read.
- Live probe: MCK Trucking board 52701 still lists job 6343. Its detail under
  `portal_user_id=1` returns 200 (8,766 bytes). `active_job_array` now contains only
  `location_type/value/state`, while `#job_title`, `#job_description` and
  `#job_requirements` hold the rendered posting. With `portal_user_id=0` its array is empty;
  the adapter's existing `=1` remains necessary.
- Cause: new template moved title/body out of the location array; parser required each row
  to carry `active_job_id` and title, so it rejected a real populated page.
- Fix: retain the legacy array parser; for a nonempty location-only array, read the explicit
  DOM fields. Reject mixed or mismatched IDs; continue to defer empty/gone pages.
- Verification: regression covers the location-only template, description and US state
  mapping; captured real MCK page parses as `CDL A Company Driver ()` with its full body
  and AL/IL/IN/KY/OH/TN states. Existing gone/wrong-ID fixture tests remain passing.

## Bullhorn

- Initial state: both catalog boards cooled: Flanders `70:D1Z9J4` (26 failures, last
  success September 19), Optomi `41:DJVM1` (18 failures, last success October 30).
- Operator live probes of both public REST search APIs return 403 (540/541-byte responses).
  The identical Optomi query from laptop returns 200, total 78, including job 39759
  `Software Engineer Intern`. Flanders times out after 35 seconds on laptop.
- Cause/blocker: Optomi is valid and the operator's access is blocked; not a broken query
  or a deleted corp token. Flanders remains inconclusive outside operator (timeout), and
  403 alone cannot justify retirement. Keep the existing error behavior. Parent should
  measure a sanctioned alternate egress for these fixed vendor hosts; do not blindly add
  a proxy rule or retire both companies.

## Production follow-up

Deploy the adapter changes only after the parent reviews them. Let the existing cooldowns
expire and scheduled shards retry; do not force a mass crawl. Track successful JOIN bodies,
Tenstreet body hydration and SC Johnson bodies after deployment. External access/routing and
catalog migrations above remain separate follow-ups. Host configuration was not changed.

## Incremental detail probes and final validation

A temporary test exercised the actual `HydratingSource.FetchNew` paths using only the
existing production identities as the seen set (read-only DB export, no data mutation).

- Intervieweb: the sole new identity was 83980,
  `/jobs/it-network-engineer-noc-83980/en/`. HTTP 200, real title
  `IT Network Operation Center Specialist`, no JSON-LD, and a complete
  `#description__body`. Added an identity-checked HTML fallback and regression test;
  rerunning the same incremental adapter probe returned 22 postings without error.
- PeopleSoft: the exact failed new openings are DC **33966** and Winnipeg **125461**.
  A second round of bounded probes resolved both to sequence 2 (details above). The
  provider-specific fix keeps the all-new-details-unreadable guard intact.

The temporary live tests are removed after capturing these results; committed tests use
local fixtures/fakes and require no external services. No debug instrumentation remains.

CodeRabbit's first completed review found two whitespace-only HTML body cases and
questioned posting identity after a Tenstreet redirect. Both HTML fallbacks now reject
trimmed-empty descriptions, with regressions. Tenstreet now validates the resolved detail
URL's host, company ID and posting ID before parsing it; a wrong-page redirect regression
covers the location-only template. Its provider-local client retains the same long timeout,
shared rate limiter and burst. Only its registry expression changes; no other provider's
transport changes.

PageUp migration follow-up is concrete: both destination hosts' `/sitemap.xml` return
HTTP 200 standard `urlset` documents with `/jobs/<title-and-location>` entries, the exact
shape the existing Clinch adapter consumes. Examples are Mader's
`/jobs/automotive-electricians-southwest-midwest-and-metro-western-australia-wa-australia`
and Armaguard's `/jobs/cash-processor-bendigo-vic-australia`. Parent can validate one
Clinch detail on each and propose replacing PageUp board 893/784 with those Clinch
hosts; no new PageUp parser or blanket WAF override is needed for these two migrations.

Both example destination detail pages also returned HTTP 200 and contain the
`job-description` container consumed by Clinch. Titles match their sitemap identities:
`Automotive Electricians - Southwest, Midwest and Metro - Western Australia, WA, Australia`
and `Cash Processor - Bendigo, VIC, Australia`. The sitemap contains 38 Mader and 26
Armaguard job URLs. These are bounded public reads, not catalog changes or crawls.

Second CodeRabbit review completed over all 21 changed files. Its two questions were
checked against the actual workspace and rejected with evidence: PeopleSoft's subsequent
`SeenRefresh` goes through `pipeline.Runner.touch`, which passes only source, external ID
and deadline to `Toucher.Touch` (no URL), so it cannot replace the recovered sequence-2
URL. The claimed missing TalentBrew production fallback is already present in the staged
`talentbrew.detail` and its matching-identity regression passes. No accepted review
finding remains; the configured two-review limit was respected.

Final checks passed: `gofmt -l .` returned no paths; `go vet ./...`; the provider
regressions; `GOMAXPROCS=2 go test -p 2 ./...`; and
`GOMAXPROCS=2 go vet -p 2 -tags=integration ./...`. An earlier unconstrained full
suite hit the unrelated `TestRegister_DuplicateEmail` 1000 ms timeout; the complete
lower-concurrency rerun passed without code changes. Both CodeRabbit invocations exited
zero with completed outcomes. No PR, push, deployment, catalog mutation or cooldown reset
was performed.
