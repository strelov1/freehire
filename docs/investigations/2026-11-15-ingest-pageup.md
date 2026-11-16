# PageUp remaining identities and Clinch metadata, 2026-11-15

## Boundary and method

Standalone follow-up to integration commit `7b06ca776f7aaf02de31f0426b0ac069104f8ef9`.
No combined commits were cherry-picked. Only Clinch parsing, tests, provider notes and
this ledger changed. No production writes, retirement, cooldown reset, full crawl,
transport changes, deployment or scheduler changes.

Read-only catalog queries ran on core through operator. Public requests ran on operator:
one exact `GET https://careers.pageuppeople.com/{id}/cw/en/search/?page-items=1000`
with `X-Requested-With: XMLHttpRequest` per remaining challenge/HTML identity (33 total),
following redirects, 30-second ceiling, at most three concurrent requests. Destination
sitemaps and one first-listed detail each were bounded separately (at most two concurrent,
30 seconds, one-second separation within each sitemap/detail pair). These are observations
from one window, not sustained throughput tests. WAF rate history can affect them.

Exact response groups from those 33 listing requests:

- HTTP 200 HTML: 743 (113033 bytes), 1041 (132412), 1081 (118862),
  885 (241456), 1080 (161835). All resolved to the destinations below.
- HTTP 202, **zero response bytes**: every other identity in that 33-row set,
  each independently requested and each with its final redirect target recorded below.
  Empty 202 is not valid JSON, an empty employer or proof of deletion.
- The seven canonical 404s were already individually measured on direct and
  Scrapling HTTP + proxy in the integration report (404, 891 bytes, no redirect).
  This pass researched official employer destinations rather than repeating those probes.

All 33 destination sitemaps returned HTTP 200. Thirty-one contain job URLs;
899 and 1023 are the two non-job sitemap cases described separately below.
Of those 31 detail samples, only 1080 was readable directly; the other 30 returned
empty HTTP 202. Each of those **same URLs** was requested once through the existing
Scrapling HTTP + proxy service: all returned HTTP 200 with explicit titles and real
`.job-description` bodies. No transport switch is included in this commit.

## Validated destinations: precise additions for parent integration

All rows below target existing provider **clinch**. Copy the **exact catalog company**;
all source rows currently have `region=''`, `hub=false`, `tenants={}`, `status=active`.
The only live Clinch catalog row at preflight was Waymo, so none of these target hosts
is already live. Recheck identities and metadata at activation time; this is evidence,
not an apply script. The existing recovery script for 893/784 is intentionally untouched.

`D` = readable direct detail. `P` = direct detail 202, same detail readable with
Scrapling HTTP + proxy. Sitemap job counts are measurements, not full-crawl guarantees.
Each link identifies the exact single detail replayed. Body bytes are the captured
`.job-description` element before sanitization (not the entire HTML response).

| PageUp | Exact catalog company | Clinch host | Listing final path/status | Sitemap jobs | Detail lane / body bytes | Captured detail |
| --- | --- | --- | --- | ---: | --- | --- |
| 743 | The Citadel | `jobs.citadel.edu` | `/` / 200 | 74 | P / 3900 | [Campus Services Officer](https://jobs.citadel.edu/jobs/campus-services-officer-charleston-south-carolina-united-states) |
| 1041 | Defence Housing Australia | `careers.dha.gov.au` | `/jobs/search` / 200 | 11 | P / 3043 | [DHA Temporary Employment Register 2026/27](https://careers.dha.gov.au/jobs/dha-temporary-employment-register-2026-27-australia-wide-australia) |
| 1081 | Savills | `careers.savills.com.au` | `/jobs/search` / 200 | 10 | P / 4371 | [Retail Manager - Myer Centre](https://careers.savills.com.au/jobs/retail-manager-myer-centre-adelaide-sa-australia) |
| 885 | University of Denver | `jobs.du.edu` | `/jobs/search` / 200 | 80 | P / 5487 | [Lifeguard](https://jobs.du.edu/jobs/lifeguard-denver-co-colorado-united-states) |
| 710 | PSA International | `psacareers.singaporepsa.com` | `/jobs/search` / 202 | 118 | P / 911 | [Trainee Lashing Supervisor [Joining bonus of $8000 for Singaporeans and PRs]](https://psacareers.singaporepsa.com/jobs/trainee-lashing-supervisor-joining-bonus-of-8000-for-singaporeans-and-prs-singapore) |
| 992 | WIN Network | `careers.wintv.com.au` | `/jobs/search` / 202 | 20 | P / 3799 | [Journalist](https://careers.wintv.com.au/jobs/journalist-mackay-qld-australia) |
| 978 | Boise State University | `jobs.boisestate.edu` | `/` / 202 | 61 | P / 1168 | [Temporary Jobs On Campus](https://jobs.boisestate.edu/jobs/temporary-jobs-on-campus-boise-idaho-united-states) |
| 988 | Duratec | `careers.duratec.com.au` | `/` / 202 | 49 | P / 4407 | [Expression of Interest – Graduate Engineers](https://careers.duratec.com.au/jobs/expression-of-interest-graduate-engineers-australia-wide-victoria-australia-sydney-nsw) |
| 870 | Marist University | `careers.marist.edu` | `/` / 202 | 41 | P / 5777 | [Part-Time Faculty, Management](https://careers.marist.edu/jobs/part-time-faculty-management-poughkeepsie-ny-new-york-united-states) |
| 998 | Mater | `careers.mater.org.au` | `/` / 202 | 145 | P / 5678 | [Registered Nurse - Endoscopy](https://careers.mater.org.au/jobs/registered-nurse-endoscopy-townsville-qld-australia) |
| 825 | Australian Health Practitioner Regulation Agency | `careers.ahpra.gov.au` | `/jobs/search` / 202 | 5 | P / 2910 | [Expressions of Interest - Aboriginal and Torres Strait Islander Opportunities](https://careers.ahpra.gov.au/jobs/expressions-of-interest-aboriginal-and-torres-strait-islander-opportunities-multiple-locations) |
| 586 | City of Moreton Bay | `careers.moretonbay.qld.gov.au` | `/` / 202 | 12 | P / 10731 | [Parks Labourer Expression Of Interest (EOI) &#124; Grounds Maintenance, Natural Areas & Trees](https://careers.moretonbay.qld.gov.au/jobs/parks-labourer-expression-of-interest-eoi-grounds-maintenance-natural-areas-trees-city-of-moreton-bay-qld-australia) |
| 960 | Northline | `careers.northline.com.au` | `/` / 202 | 35 | P / 5253 | [Storeperson](https://careers.northline.com.au/jobs/storeperson-vic-melbourne-vic-australia-qld-brisbane-queensland-wa-perth-western-australia-nsw-nsw-sydney) |
| 924 | Charles Sturt University | `external-jobs.csu.edu.au` | `/` / 202 | 26 | P / 2943 | [Casual Employment Register](https://external-jobs.csu.edu.au/jobs/casual-employment-register-albury-wodonga-nsw-australia-bathurst-dubbo-orange-port-macquarie-wagga-wagga) |
| 1027 | North Idaho College | `north-idaho-college-careers.career-pages.com` | `/` / 202 | 26 | P / 1280 | [Adjunct Faculty Talent Pool - Social & Behavioral Sciences](https://north-idaho-college-careers.career-pages.com/jobs/adjunct-faculty-talent-pool-social-behavioral-sciences-coeur-d-alene-idaho-united-states) |
| 820 | Drexel University | `careers.drexel.edu` | `/` / 202 | 46 | P / 4231 | [Exam Proctor](https://careers.drexel.edu/jobs/exam-proctor-university-city-philadelphia-pa-pennsylvania-united-states) |
| 902 | Arts Centre Melbourne | `careers.artscentremelbourne.com.au` | `/` / 202 | 4 | P / 2429 | [Expressions of Interest - Staging](https://careers.artscentremelbourne.com.au/jobs/expressions-of-interest-staging-melbourne-cbd-vic-australia) |
| 1072 | Whiddon | `careers.whiddon.com.au` | `/` / 202 | 102 | P / 448 | [Register your Interest to Work at Whiddon](https://careers.whiddon.com.au/jobs/register-your-interest-to-work-at-whiddon-kelso-nsw-australia-mudgee-laurieton-wingham-belmont-largs-redhead-bourke-moree-narrabri-wee-waa-casino-grafton-kyogle-maclean-tweed-heads-temora-gle) |
| 1000 | Knox City Council | `careers.knox.vic.gov.au` | `/` / 202 | 4 | P / 10791 | [Program Leader - Early Years Hub](https://careers.knox.vic.gov.au/jobs/program-leader-early-years-hub-wantirna-south-children-and-family-centre-vic-australia) |
| 449 | Linfox | `careers.linfox.com` | `/` / 202 | 65 | P / 3105 | [FIFO MC Driver](https://careers.linfox.com/jobs/fifo-mc-driver-perth-wa-australia-wa-australia) |
| 867 | University of San Diego | `jobs.sandiego.edu` | `/` / 202 | 72 | P / 1214 | [Lecturer - College of Arts and Sciences, Biology](https://jobs.sandiego.edu/jobs/lecturer-college-of-arts-and-sciences-biology-san-diego-california-united-states) |
| 987 | Berry Street Yooralla | `jobs.yooralla.com.au` | `/` / 202 | 9 | P / 7040 | [Behaviour Support Practitioner - Core](https://jobs.yooralla.com.au/jobs/behaviour-support-practitioner-core-melbourne-cbd-vic-australia) |
| 709 | CIMIC Group | `career.leightonasia.com` | `/` / 202 | 53 | P / 1732 | [MEP Commercial Manager](https://career.leightonasia.com/jobs/mep-commercial-manager-hong-kong) |
| 530 | World Vision Australia | `careers.worldvision.com.au` | `/` / 202 | 15 | P / 1950 | [Expression of Interest](https://careers.worldvision.com.au/jobs/expression-of-interest-australia-wide-victoria-australia) |
| 923 | University of Tasmania | `careers.utas.edu.au` | `/` / 202 | 15 | P / 1031 | [Aboriginal and Torres Strait Islander Expression of Interest](https://careers.utas.edu.au/jobs/aboriginal-and-torres-strait-islander-expression-of-interest-hobart-tas-australia-launceston-burnie-sydney-nsw-melbourne-vic-west-coast-tasmania-not-applicable) |
| 951 | Amana Living | `jobs.amanaliving.com.au` | `/` / 202 | 15 | P / 4018 | [Community Support Worker](https://jobs.amanaliving.com.au/jobs/community-support-worker-northern-suburbs-wa-australia-ec6917d1-43d6-4163-8ab3-795f124d0870) |
| 1080 | Schindler Lifts | `schindler-lifts-australia-careers.careerpages.rec-marketing.dc2.pageuppeople.com` | `/jobs/search` / 200 | 31 | D / 4827 | [Service Sales Consultant - NSW](https://schindler-lifts-australia-careers.careerpages.rec-marketing.dc2.pageuppeople.com/jobs/service-sales-consultant-nsw-sydney-nsw-australia) |
| 1098 | Device Technologies | `careers.device.com.au` | `/` / 202 | 20 | P / 7192 | [Product Specialist/Territory Manager - INR](https://careers.device.com.au/jobs/product-specialist-territory-manager-inr-sydney-nsw-australia) |
| 671 | United Nations Children's Fund (UNICEF) | `jobs.unicef.org` | `/` / 202 | 191 | P / 11007 | [National consultant Face-to-Face (F2F) Fundraiser for Croatia Country Office(m/f), Zagreb, Croatia (for Croatian Nationals only)](https://jobs.unicef.org/jobs/national-consultant-face-to-face-f2f-fundraiser-for-croatia-country-office-m-f-zagreb-croatia-for-croatian-nationals-only-croatia) |
| 695 | Kenyon College | `careers.kenyon.edu` | `/` / 202 | 26 | P / 2970 | [Coordinator of Muslim Life](https://careers.kenyon.edu/jobs/coordinator-of-muslim-life-gambier-oh-ohio-united-states) |
| 724 | Life Without Barriers | `careers.lwb.org.au` | `/` / 202 | 36 | P / 4325 | [Intensive Therapeutic Care Worker. Casual - Central Coast](https://careers.lwb.org.au/jobs/intensive-therapeutic-care-worker-casual-central-coast-gosford-central-coast-nsw-australia) |
| 1138 | Fire and Rescue NSW | `careers.fire.nsw.gov.au` | canonical 404 (prior probe) | 27 | D / 4406 | [On-Call Firefighter - RN1 - 279 Dorrigo](https://careers.fire.nsw.gov.au/jobs/on-call-firefighter-rn1-279-dorrigo-port-macquarie-nsw-australia-8d9050f0-73bd-4ba0-b320-6dafe43aee2c) |
| 1155 | TAFE NSW | `careers.tafensw.edu.au` | canonical 404 (prior probe) | 42 | D / 6705 | [Education Support Officer](https://careers.tafensw.edu.au/jobs/education-support-officer-lake-macquarie-nsw-australia) |
| 1197 | NSW State Emergency Service | `careers.ses.nsw.gov.au` | canonical 404 (prior probe) | 1 | D / 8320 | [Coordinator, Product Management, Clerk Grade 7/8](https://careers.ses.nsw.gov.au/jobs/coordinator-product-management-clerk-grade-7-8-rhodes-nsw-australia-wollongong) |
| 1199 | NSW Rural Fire Service | `careers.rfs.nsw.gov.au` | canonical 404 (prior probe) | 12 | D / 8659 | [Senior Advisor Leadership Development](https://careers.rfs.nsw.gov.au/jobs/senior-advisor-leadership-development-sydney-olympic-park-nsw-australia) |
| 1192 | NSW Independent Commission Against Corruption | `careers.icac.nsw.gov.au` | canonical 404 (prior probe) | 1 | D / 4358 | [Lawyer](https://careers.icac.nsw.gov.au/jobs/lawyer-hybrid-sydney-nsw-australia-c4fa9a8e-e671-4425-be7d-9d131de8e81e) |

The 36 rows above passed a temporary local replay through **actual `NewClinch.Fetch`**:
each full captured sitemap, its first listed real detail, and fake failures for other
details (no public requests during replay). Asserted emitted job count equals sitemap
job count, sample URL/external ID/company remain intact, exact explicit title/location
match, and description is nonempty. Removed the temporary replay test after measurement;
the five permanent real fixtures cover metadata theme variants and stable UUID identity.
These replays prove adapter compatibility, not production body coverage.

Previously validated 893 (**Mader Group**, `careers.madergroup.com.au`, 38 sitemap jobs)
and 784 (**Armaguard Group**,
`armaguard-careers.careerpages.rec-marketing.dc2.pageuppeople.com`, 26 sitemap jobs)
retain their integration-report evidence. This commit fixes and permanently regresses
both real details: `Cash Processor` / `Bendigo, VIC`, and
`Automotive Electricians - Southwest, Midwest and Metro` / `Western Australia, WA`.
Their reviewable recovery script remains owned by the parent.

## Remaining exact blockers: retain history and catalog

| PageUp | Catalog employer | Individual observation and action |
| --- | --- | --- |
| 899 | genU (Activ destination) | Listing resolves to `https://careers.activ.asn.au/`, HTTP 202, 0 bytes. `https://careers.activ.asn.au/sitemap.xml` returns 200 with a genuinely empty XML urlset. No listed detail exists to validate. Same destination requested once through Scrapling HTTP + proxy returns 200 (79627 bytes), final `/jobs/search`, explicit **No results found**, no job links, and genU branding. The old Activ host is a readable empty listing, not an unresolved JSON bug. **Block activation:** there is no listed detail to validate; keep history and require an actual new vacancy or verified replacement catalog before adding a Clinch board. |
| 1023 | 1023 (North Memorial destination) | Listing resolves to `https://jobs.northmemorial.com/nmh/en-us`, HTTP 202, 0 bytes. Root sitemap returns 200 but its only URL is `http://careers.pageuppeople.com/`; there are no `/jobs/` entries. Same listing through Scrapling HTTP + proxy returns 200 (35784 bytes), **174 Current opportunities**, 10 distinct `a.job-link` URLs duplicated in responsive markup. It remains a legacy PageUp theme, with URLs `/nmh/en-us/job/514883/environmental-services-aide`, not Clinch `/jobs/` slugs. **Block activation:** existing PageUp hardcodes the canonical host and `/cw/en/job/` ID regex; neither current adapter supports this custom path. Root-sitemap Clinch would silently emit zero. Needs a separately scoped PageUp host/path adaptation using this measured protocol, not a new speculative crawler. |
| 1034 | Department of Primary Industries and Regional Development | Canonical 404 retained. Official [DPIRD careers page](https://www.dpird.nsw.gov.au/about-us/who-we-are/career-opportunities) fetched through Scrapling HTTP + proxy returns 200 and links the filtered I Work for NSW URL below. That exact listing returns operator-direct 403; Scrapling HTTP + proxy returns 200 **security-check HTML**, not vacancies. **Block activation:** a real accessible employer listing and existing-adapter compatibility; no speculative crawler or retirement. |
| 1245 | 1245 | Canonical 404 retained. Bounded read-only lookup of five historical job URLs still gives company `1245`. Public primary-source [job 492217](https://careers.pageuppeople.com/mob/1245/cw/en/job/492217/band-member-drum-kit-percussion) and [job 492220](https://careers.pageuppeople.com/mob/1245/cw/en/job/492220/product-owner-user-security) identify **NSW Police Force**. Its [official recruitment page](https://www.police.nsw.gov.au/recruitment/careers_and_salary/aboriginal_and_torres_strait_islander_recruitment) points current vacancies to I Work for NSW. I Work for NSW root returns HTTP 403 through the existing Scrapling HTTP + proxy lane. **Block activation:** an accessible, employer-scoped listing and adapter validation; also review the incorrect numeric company label separately rather than propagating it into a new board. |

DPIRD's exact official link:
`https://iworkfor.nsw.gov.au/jobs/all-keywords/industry-nsw/department-of-industry---primary-industries/all-categories/all-locations/all-worktypes?divisionid=26&agenciesid=6598`.
The proxy final URL is `/security-check?returnTo=...` on that same host; a 200 there is
an access blocker, not a successful listing. Indexed PageUp search results still exist
for NSW agencies; search-engine caches are not evidence their canonical route works now.

Official provenance for the five recovered 404 identities:

- 1192: [ICAC employment](https://www.icac.nsw.gov.au/about-the-nsw-icac/employment-opportunities)
  proxy 200, current-vacancies link to `https://careers.icac.nsw.gov.au/jobs/search`.
- 1138: [Fire search](https://careers.fire.nsw.gov.au/jobs/search), official NSW host;
  sitemap/detail 200, exact job UUID retained.
- 1155: [TAFE careers](https://www.tafensw.edu.au/about/careers-and-jobs) and
  [current opportunities](https://careers.tafensw.edu.au/jobs/search), official EDU host.
- 1197: [SES careers](https://www.ses.nsw.gov.au/about-us/careers), HTTP 200;
  explicit links to `https://careers.ses.nsw.gov.au/jobs/search`.
- 1199: [RFS careers](https://www.rfs.nsw.gov.au/about-us/careers) redirects to
  `https://careers.rfs.nsw.gov.au/`.

## Parser behavior and activation conditions

Readable pages now override slug guesses with `.job-title` and explicit location
components. Physical locations take priority, then `.job-location` spans, then the
explicit location-group theme; repeated responsive copies are deduplicated. Every
explicit location is retained verbatim apart from whitespace. No place-name inference
or dictionary changes were introduced. Generic h1 and OG/document titles are unsuitable
because they contain page chrome or combined title/geography.

Sitemap URL, slug/UUID identity, catalog company, lastmod date, emitted counts, and
challenge latch remain intact. A challenge stops further details for that run;
ordinary failures keep the posting and slug fallback. Existing `fullBoardListing` /
`emptyBoardListing` declarations are unchanged. An unhydrated fallback can still have
imperfect slug metadata; this fix cannot manufacture fields behind a WAF.

Before activation: integrate/release parser changes; recheck destination metadata through
`boardcatalog` and use `cmd/add-board` (default dry run) for only validated destinations.
Thirty validated targets required the proxy lane for this sample; the existing production
Clinch detail route must demonstrate readable hydration under normal pacing before claiming
recovery. This commit deliberately changes no shared transport. A direct sitemap-only run
is possible but is **not** evidence that hydrated metadata/bodies work in production.
Keep old PageUp rows and all job/application identities. After separately approved complete
destination runs, compare coverage/bodies and deduplication through normal identity
mechanisms, then separately review retiring only proven replaced rows. No cooldown reset
or mass crawl is needed. Four blocked rows above must not be activated from this evidence.

## Verification

Permanent real-fixture regressions initially failed on Armaguard and Mader and pass after
the metadata fix. Existing challenge-latch, detail-error, sitemap count/identity and slug
fallback regressions pass. Additional real fixtures cover repeated location spans and
Fire's location-group layout/UUID. Simplify pass retained one parser path and existing
DOM helpers; no new abstraction, dependency or crawler.

Passed with `GOMAXPROCS=2` and `-p 2`: `go vet ./...`, `go test ./...`,
and `go vet -tags=integration ./...`. Gofmt, targeted Clinch tests, all 36 captured
Clinch destination replays, size/relative-link checks, staged whitespace check and
Gitleaks passed. No database-backed integration suite or production crawl was run.

CodeRabbit CLI 0.9.5, `review --agent --uncommitted --fresh`, exited zero with
`outcome=completed`, 11 reviewed files and zero findings. A final documentation addendum
below records later bounded North Memorial/empty-site probes; parser behavior was unchanged.

### Final North Memorial protocol probe

The existing Scrapling HTTP + proxy route returned 200 for
`https://jobs.northmemorial.com/nmh/en-us/search/?page-items=1000` with the XHR header:
146491 response bytes, real JSON keys `results/filter/page/pageitems/count`,
`page=1`, `pageitems=1000`, **count=184** and **184 distinct job links**. The earlier
landing page displayed 174, so the two responses are separate snapshots, not an asserted
count agreement. One listed detail, `/nmh/en-us/job/514883/environmental-services-aide`,
returned 200 (32468 bytes) with a real `#job-details` element (6156 serialized bytes).
A temporary replay through actual `NewPageUp.Fetch` with this captured envelope emits
**zero jobs without error**, confirming the custom-path regex incompatibility. Removed
the temporary replay after measurement. Do not activate Clinch here or call the 202 an
employer disappearance; a future PageUp path fix has a measured JSON/detail protocol.
