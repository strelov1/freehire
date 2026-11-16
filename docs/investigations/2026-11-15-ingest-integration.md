# Ingest integration, 2026-11-15

## Integration boundary

Branch `t3/repair-ingest-integrations` starts at `081f81e53ad37141101a34293ee8c87f138d6f2a`.
The six supplied patches are preserved separately and in order:

| Original | Integrated | Scope |
| --- | --- | --- |
| a04e2607b | 30ffc4b12 | Mynavi and Optius detail parsing |
| 4a6065ebc | b588ed978 | Foxconn readable/bodyless distinction, Freshteam HTML, ADP MyJobs incremental hydration |
| 5d8b8ac89 | b786e0f15 | JOIN GraphQL, Intervieweb HTML, PeopleSoft sequence 2, TalentBrew HTML, Tenstreet HTML/identity |
| 270b04100 | 76da4869b | Gradcracker, InfoJobsBR and SpainJobs measured Scrapling routes |
| c8a243647 | 4e8624825 | UKG hydration, Teamtailor bounded repetition, live-identity health SQL and actual success timestamps |
| a6ecaa687 | 9aa8abbe5 | Body-refresh production prerequisite |

No overlap required resolution. Disk exhaustion interrupted the second cherry-pick after
writing its files but before writing the index. Its staged patch ID was checked against
the original (`69385cc115a993079175dd48424f22a9cb680100`), then committed with the original
message and author; the remaining cherry-pick was applied normally. Hooks were bypassed
only for restoring the supplied commits; combined checks are recorded below.

Read all three ledgers: [aggregators](2026-11-15-ingest-aggregators.md),
[fleet](2026-11-15-ingest-fleet.md), and [ATS](2026-11-15-ingest-ats.md).
The [routing follow-up](2026-11-15-ingest-routing.md) was subsequently integrated when
its owner released shared routing ownership. Its actual parser probes verified Gradcracker
(two listings/detail), InfoJobsBR (session and distinct fragments/detail), and SpainJobs
(sitemaps/detail). The [fleet follow-up](2026-11-15-ingest-fleet-followup.md) was then integrated, including
UKG/Teamtailor and SQL/status accounting. HigherMe/RWB and ops preparation remain a
separate task. No production data, service, cooldown, scheduler or host configuration
was changed.
No full crawl was launched. Mindsight and OpenCATS external blockers were not reprobed.

## Remaining public vendor routes

Measurements used operator's existing environment without exposing credentials. Each test
was one exact public request, with existing proxy/service configuration. Scrapling HTTP is
the existing Chrome fingerprint lane. Browser navigation cannot send Senior's POST; no
unsupported browser POST workaround was attempted. Initial curl-output framing mistakes
were discarded and direct/proxy measurements repeated with corrected framing.

Bullhorn URL: `https://public-rest41.bullhornstaffing.com/rest-services/DJVM1/search/JobOrder`
with the adapter's exact `count=500`, `start=0`, `query=(isOpen:1)`, `sort=-dateAdded` and
`fields=id,title,publicDescription,address(city,state,countryName),dateAdded,employmentType`.
Cluster 70 substitutes `public-rest70` and `D1Z9J4` only.

| Public request | Direct operator | Existing proxy | Scrapling HTTP direct | Scrapling HTTP + proxy |
| --- | --- | --- | --- | --- |
| Bullhorn Optomi 41:DJVM1 | 403, 540 bytes | 200, JSON total/data 78 | 403, 540 bytes | 200, JSON total/data 78 |
| Bullhorn Flanders 70:D1Z9J4 | 403, 540 bytes | timeout after 40 s, no response | 403, 540 bytes | service request failed; no vendor result established |
| Senior Intelbras profile POST | 403, empty | 403, empty | 403, empty | 200, valid profileId |

Optomi's earlier laptop 200/total 78 corroborates IP refusal. Both successful operator
proxy responses are 552395 bytes with real title/body/address fields. **Implemented:** rebuild Bullhorn with its existing proxy client and retain normal pacing,
errors and pagination. A provider-wide route still does not establish Flanders recovery;
keep that failure visible. The measured proxy route is implemented below.

Senior's exact endpoint is
`https://platform.senior.com.br/t/senior.com.br/bridge/1.0/anonymous/rest/hcm/careersmanagercandidate/actions/getProfileIdBySubdomain`,
POST JSON `{"subdomain":"intelbras"}`, `Content-Type: application/json`.
Fingerprint plus proxy resolves profile `af20908f-a8d3-4133-9537-c6e32b13d5aa`.
Plain proxy alone is insufficient. A follow-up POST to `/queries/searchVacancies` with
the adapter's exact first-page body (`page=0`, `size=50`, empty filter/localizations,
the resolved company ID) returns 200 JSON, 38908 bytes and `totalPages=1`.
One `/queries/findVacancyById` POST for listed ID
`4fe79388-242a-4adb-a23c-e4f366cb8320` returns 200 JSON (14001 bytes) with an 8897-character
description. An earlier size-20 bounded search also succeeded; the size-50 repeat matches
the production adapter. **Implemented:** route Senior through
Scrapling HTTP + proxy, preserving its POST body/headers and existing request limits.
This is evidence for Intelbras, not proof that every tenant or sustained crawl succeeds.

## PageUp disposition

The ATS ledger's 42 failing identities group into 26 challenges, 9 HTML-as-JSON errors,
and 7 404s. This is a historical health snapshot, not a current active-board count.
No retirement follows from an error group alone.

Bounded sample: replayed the exact XHR listing request for **all seven** 404 identities,
plus WAF samples 695 and 724 and valid control 889, on operator direct and Scrapling HTTP
with proxy. No detail crawl or scan of all 26 challenged boards was performed.

- **1245, 1192, 1138, 1155, 1034, 1199, 1197:** both lanes return 404, same 891-byte
  HTML response, no migration redirect. These are unavailable canonical routes, not
  evidence of deleted employers. Keep catalog/history; blocker is identifying each
  employer's current official destination. Proxy switching is not a demonstrated fix.
- **695 (Kenyon):** both lanes follow a redirect to `https://careers.kenyon.edu/jobs/search`,
  HTTP 200 HTML (~140773 bytes). **724 (Life Without Barriers):** both lanes reach
  `https://careers.lwb.org.au/jobs/search`, HTTP 200 HTML (~202707 bytes).
  These are migration candidates requiring their own sitemap/detail validation before
  catalog changes. The historical WAF label does not describe today's final page.
- **893:** already verified redirect to Mader (see below). This is another historical
  challenge row that is now a migration, so a blanket challenge transport rule is wrong.
- **Remaining 23 challenge identities:** 743, 1041, 1081, 885, 710, 992, 978, 988, 870,
  998, 825, 586, 960, 1023, 924, 1027, 820, 902, 1072, 1000, 449, 867, 987.
  Retain as unresolved access-or-migration candidates; not sampled in this pass. No
  retirement or route change is supported for them by another board's response.
- **889 control:** both lanes return real JSON, count 394, 393263 bytes. The canonical
  XHR protocol still works. A global switch to HTML parsing would break working tenants.
- **Nine HTML decode cases:** 784 is confirmed below; 709, 530, 923, 951, 899, 1080,
  1098 and 671 remain individual migration investigations, not JSON repair candidates.

## Reviewable PageUp to Clinch recovery

Read `boardcatalog/AGENTS.md`, `cmd/add-board/main.go` and its worker documentation (no
separate add-board worker entry exists). Read-only catalog preflight confirms both source
rows active, company names exactly **Mader Group** and **Armaguard Group**, empty regions,
`hub=false`, `tenants={}`. Neither Clinch destination exists as a live catalog row.

| PageUp board | Verified Clinch host | Sitemap jobs | One captured detail replay |
| --- | --- | --- | --- |
| 893 | careers.madergroup.com.au | 38 | Automotive Electricians Southwest Midwest And Metro; body 2094 bytes |
| 784 | armaguard-careers.careerpages.rec-marketing.dc2.pageuppeople.com | 26 | body 2399 bytes; incorrect slug-derived title/location |

Both sitemaps and details were captured by the ATS investigation with HTTP 200. A temporary
local test replayed each **actual `NewClinch(...).Fetch`** with a one-entry subset of its
captured sitemap and its real HTML detail (no extra public requests). Identity URL,
external ID, company and body assertions passed. The test was removed after measurement.

**Activation blocker:** Clinch currently splits `cash-processor-bendigo-vic-australia` into
title `Cash` and location `Processor Bendigo Vic, Australia`. The real HTML explicitly says
`Cash Processor` and `Bendigo, VIC`. Mader parses a plausible title/location but loses its
original punctuation. Thus sitemap/body compatibility is established, but these two
samples do not justify declaring a production-ready catalog move. Correct and regress
Clinch's title/location extraction (including sitemap-only WAF fallback) before activation.
This task does not alter its general location dictionary behavior speculatively.

Prepared [pageup-clinch-recovery.py](pageup-clinch-recovery.py) uses the established
`cmd/add-board` writer, never raw inserts. Run from repository root with the intended
`DATABASE_URL` already set:

```sh
python3 docs/investigations/pageup-clinch-recovery.py
# Only after the Clinch metadata blocker is resolved and release approved:
python3 docs/investigations/pageup-clinch-recovery.py --apply
```

Default execution makes a read-only preflight and prints exact dry-run add commands.
Apply adds only absent destinations. It preserves exact source company names, rejects
conflicting destinations/unexpected source metadata before any write, and skips matching
live destinations on repeat. Unique catalog identity remains the concurrent-insert
backstop; a racing insert fails safely and a rerun reconciles. Tests cover repeat execution,
company conflicts and missing/unexpected source metadata. It was not applied to production.

The script deliberately leaves both PageUp rows and every job/application untouched.
Activation creates a new source namespace; do not rewrite old job IDs or application
references. After a separately approved complete destination run, compare sitemap identities,
real bodies and existing jobs, resolve duplicates through normal job identity mechanisms,
then review retiring **only** 893/784 via `cmd/add-board --retire`. Retirement soft-closes
old postings and removes their health; it never deletes job history. This closure requires
its own coverage evidence and approval, so is not automated here. Keep the old boards
until that evidence exists, accepting visible errors during the transition.

## Release checks and limits

Parent must integrate the separately delegated RWB/HigherMe and ops preparation, then
rerun whole-module checks before release.
Then use normal release deployment and natural cooldown expiry, without a mass crawl or
cooldown reset. Observe new hydrated bodies for the changed providers, JOIN pagination,
ADP MyJobs/UKG completion and request reduction, Foxconn readable versus bankable counts, and
provider-specific route acceptance. Track actual successful boards and interruptions rather
than green cooldown-only units. Host changes, if any, also belong in `freehire-ops`.

**Release prerequisite:** `BODY_REFRESH_DAYS` and `BODY_REFRESH_SLICE` are absent on
operator; periodic body refresh is disabled. UKG and ADP MyJobs retain the repository's
opt-in refresh mechanism, but their new known-ID paths will not periodically fetch edited
bodies until the separately delegated ops preparation enables and budgets that policy.
Do not claim those edits refresh periodically with the current production configuration.

Additional integration changes add Bullhorn's existing proxy route and Senior's measured
Scrapling HTTP + proxy route. Their regressions failed before routing and passed after;
Senior's fake wire regression exercises all three actual adapter POSTs, JSON bodies,
headers, mode/proxy flags, and the resulting hydrated job. No provider semantics changed.
Without the corresponding proxy/service env configuration, the existing direct routes remain.

Combined local verification (the first pass covered the original three commits; final
validation after the routing and fleet commits is recorded below):

- `GOMAXPROCS=2 go vet -p 2 ./...`: passed.
- `GOMAXPROCS=2 go test -p 2 ./...`: passed, including the sources package.
- `GOMAXPROCS=2 go vet -p 2 -tags=integration ./...`: passed.
- Targeted regressions for all ten changed providers: passed.
- `GOMAXPROCS=2 gofmt -l .`: no paths; existing Go changes needed no formatting edits.
- Python recovery tests: five passed. Ruff check and format check passed.
- Recovery script's actual default preflight on core: exit 0, two exact add commands,
  read-only catalog access only. The initial connection attempt revealed that psql treats
  a URI in `PGDATABASE` as a literal name; fixed by using its intended `--dbname` API,
  capturing failures and reporting a generic error to avoid exposing connection secrets.
- `pnpm check:size`, `git diff --check`, staged diff check, and Gitleaks over the three
  integrated commits and own follow-up diff: passed.
- Simplify pass: retained the existing provider APIs; the recovery script only checks
  the two explicit identities and invokes the established writer. No extra abstraction,
  dependency, full crawler or speculative transport was added.

CodeRabbit round 1 completed successfully over the original three commits and then-current
follow-up (42 files, six findings). One accepted defect: Freshteam accepted `<div></div>`
as a rendered posting body. A regression failed before the fix and now requires an
unreadable marker. The fallback checks retained text before accepting the body.

The other findings were checked against code:

- Inactive Clinch rows cannot be reassigned/reactivated: `InsertBoard` is an INSERT with
  no conflict-update clause. Retired/rejected history remains intact; a new live row is
  intentionally allowed by the catalog identity rules. The script's active-only identity
  preflight matches those rules.
- ADP edited-body refresh is supported by `ExistingExternalIDsByBoard`'s stale-body
  exclusion and `cmd/ingest/store.go` parameters, but disabled in current production.
  Accepted as the explicit ops release prerequisite above, not silently assumed enabled.
- PeopleSoft's `pipeline.Runner.touch` passes source/external ID/deadline only to `Touch`;
  the stored sequence-2 URL is not overwritten by `SeenRefresh`.
- Optius's regular expression already accepts a single closing brace. The cited final-field
  regression passes unchanged; no regex edit was warranted.
- `NewJoin` constructs `rateLimitedJSONPoster` with one shared limiter for its listing
  and detail calls; `TestJoinIsPaced` passes. The registry does not bypass pacing.

Final review includes later routing/fleet integration and the connection-URI/script test
fixes made during round 1. Its outcome is recorded below. The database-backed integration
suite was not run; integration-tag vet proves compilation, not database behavior.
No claim is made that production ingests are repaired.

## Final combined review and verification

CodeRabbit round 2 used `--fresh` against the original base, completed successfully,
and reviewed 67 files across all six integrated commits and the follow-up. Four findings:

- Accepted: clearing cooldown counters after a quiet-board failure could restore internal
  freshness without success. A real Postgres regression reproduced this through
  `RecordBoardFailure` then `ClearProviderCooldowns`. The rollup now also requires no error
  timestamp after the latest success. SQL source was updated and sqlc regenerated.
  The regression passes after the fix, including a subsequent real successful retry.
- Rejected: JOIN pacing question repeats round 1; constructor-owned shared limiter remains.
- Rejected: Tenstreet listing calls `GetHTMLResolved`, which waits on the same
  `tenstreetClient.limiter` as resolved detail reads. No separate/unpaced budget exists.
- Rejected: retirement wording. `QueriesRepository.Retire` explicitly calls
  `DeleteBoardHealth`; `boardcatalog/AGENTS.md` documents that behavior. Scoped health
  reporting retains existing excluded history, but explicit retirement still deletes its
  health row. The ledger accurately distinguishes these operations.

Both reviews exited zero with completed outcomes. No third review was run, respecting the
two-round limit. The final SQL correction was manually inspected and executed locally.

Targeted database tests passed on this checkout's isolated Postgres:
`TestBoardHealthCatalogScope`, `TestProviderHealthQuietFreshnessKeepsActualSuccess`,
`TestListChronicBoards*`, and `TestQuietBoardsCountAsFresh`. Handler
`TestIngestStatusEndpoint` also passed with the final generated query. These are local
writes to throwaway test databases, not production mutations. The full tagged integration
suite and the full verify-freehire UI journey were not run; the verification skill was read,
and this task uses bounded public probes plus adapter/SQL/endpoint checks without a full crawl.

Final checks after all six commits and both accepted review fixes passed:
`GOMAXPROCS=2 go vet -p 2 ./...`, `GOMAXPROCS=2 go test -p 2 ./...`, and
`GOMAXPROCS=2 go vet -p 2 -tags=integration ./...`. Golangci-lint over changes since
integrated HEAD reports zero issues. Gofmt, sqlc regeneration, Ruff, five Python tests,
link/size checks, whitespace checks and the final follow-up Gitleaks scan passed.
All six supplied commits retain matching stable patch IDs. Production recovery remains
unclaimed; the separately delegated ops/RWB/HigherMe work and the catalog blockers above
remain release follow-ups.

## Parent integration: RWB and HigherMe

Integrated recovery commit `9c11d750090ae6213716cdd799ec4053e033cf85` as
`efe9b132f`, then applied its measured RWB browser/direct route to the shared
Scrapling registry. The existing route regression was first run with the expected
RWB entry but no implementation and failed; the routing, RWB and HigherMe tests
pass after registration. HigherMe retains its original interval: admission of
retries is corrected, but sustained production acceptance is not established.

A bounded read-only production query completed for stored catalog sizes and
hash-slice upper bounds (no job_activity join):

| Provider | Stored rows | Largest 30-way slice | Largest 90-way slice |
| --- | --- | --- | --- |
| UKG | 567246 | 19162 | 6520 |
| ADP MyJobs | 483912 | 16344 | 5575 |

These include closed and recently hydrated rows, so they are conservative counts
of potentially eligible identities, not measured detail requests. At UKG's
existing approximately five requests/second, 19162 detail reads alone take at
least 3832 seconds, without listing calls, latency or retries. Thus thirty slices
is not a demonstrated safe activation for a constrained run. Ninety slices lowers
the bound but does not establish run headroom or refresh latency suitability.
A separate current-slice query joining activity timestamps hit its 20-second
statement timeout; no stale/open count was obtained. Do not interpret that timeout
as zero eligible records. Periodic refresh remains disabled in production; the
prepared ops patch is uninstalled and its proposed slice count requires budgeting.
No production mutations or full crawl were performed.

Parent verification after integrating RWB/HigherMe and the RWB route: full tests
passed (267 package results), `go vet -p 2 ./...` and
`go vet -p 2 -tags=integration ./...` passed with GOMAXPROCS=2. The final targeted
Scrapling/RWB/HigherMe run passed after formatting. Size and whitespace checks
passed. The small route registration was manually inspected; no further combined
CodeRabbit round was started after the existing two completed rounds. The
PageUp/Clinch follow-up remains separate and must be integrated and checked before
claiming the complete recovery branch ready.
