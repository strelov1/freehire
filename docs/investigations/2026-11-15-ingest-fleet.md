# Fleet ingest investigation, 2026-11-15

Read-only production inspection began at 22:58 UTC on operator (89.167.94.146),
with database queries through `ssh core`, `sudo -u postgres psql -d hire`, inside
`BEGIN READ ONLY`. No production state, cooldown, unit, or deployment was changed.
Counts below are observations at different instants, not a frozen snapshot.

## Provider ledger

| Provider | Initial state and actual error | Live probe from operator | Cause and disposition |
| --- | --- | --- | --- |
| Foxconn | Status down; health has 7 failures, last success November 10 11:02 UTC, last error November 14 21:11: `none of 51 new postings could be read`. Later scheduler runs skip its cooldown and exit successfully. | Listing 200, 979 records. Database holds 929 IDs (stored with leading `:`); 51 current IDs are unbanked. Three unbanked detail probes return 200, correct identity and null duties; the first has all `desc_1` through `desc_11` null. A known detail has a real body. | Successfully read, bodyless vacancies were counted as request failures. The adapter discarded even the known-record refreshes whenever every new record lacked a body. Fixed: count successfully decoded, identity-matching detail responses separately from bankable records. Empty descriptions remain excluded; transport and identity failures still fail a wholly unreadable batch. |
| RWB | Down; health has 4 failures, last success November 14 12:27 UTC. Listing returns HTTP 498. | Both `limit=1` and the exact adapter URL (`limit=200&offset=0`) return 498. Body contains `/__wbaas/challenges/antibot/__static/v2/index-DL5hnmG4.js`, not vacancy JSON. | Provider antibot challenge on operator IP. No schema change established. Requires a measured challenge-capable fetch lane before any transport change; do not reinterpret 498 as an empty board. |
| HigherMe | Failed unit, six page-one 403s at 22:23 UTC; 55 health rows, 46 quiet, four cooled, actual latest success 14:20 UTC. | Failed brand `5d3a7fbd62577`, same query and `limit=100`, returns 200 with 110626 bytes outside the run. `limit=1` also returns 200. | Transient run-time edge refusal, consistent with the provider note's measured rate-based WAF. Existing shared pacer starts one request each 3 seconds, faster than the documented ~70 requests/six minutes refusal point. No persistent endpoint failure. Parent should investigate aggregate concurrent traffic and lower HigherMe's shared pace with a bounded run; no shared pacer change in this batch. |
| Freshteam | Failed unit: 1317 ingested, 3823 unreadable, one interruption, truncated at 21:10 UTC; many boards show all listed details unreadable. | `10times` listing returns 200 and links `/jobs/QjrN6azL0OS-/technical-product-manager`. Detail returns 200, title/location/body present, no `JobPosting`; page ends with `Liquid error: undefined method public_fields for nil:NilClass`. | Provider template defect suppresses structured metadata while rendering the job. Fixed a fallback scoped to `job-details-header` and `job-details-content`: retain all rendered body sections, exclude separate Liquid error nodes, retain displayed title/location, and leave unavailable dates unset. Missing job structure still produces an unreadable marker. This fixes a confirmed unreadable case; the remaining run deadline is separate. |
| SmartRecruiters | Previous run failed/truncated at 21:56: 128303 ingested, one failure, 4919 unreadable, two interruptions; next run active. | CHECK24 listing returns 401; later health shows 13921 rows, one cooled, 13869 quiet, actual latest success 23:01 UTC. | Isolated tenant refusal plus incomplete run, not a fleet-wide API outage. Preserve CHECK24's error; confirm whether employer disabled public API or moved board. Investigate unreadable details and remaining long boards separately before changing provider throughput. |
| Taleo | Failed/truncated at 22:15: 46666 ingested, one failure, four interruptions. IAMGOLD page-one `searchjobs` returned HTML to the JSON decoder (`invalid character '<'`). Hyatt and PruittHealth board refreshes hit context deadline; Zions listings also produced missing-title drafts. | `aa067s02.taleo.net/ex` careersection returns 200. IAMGOLD session GET publishes portal 101430233; subsequent search POST returns 200 `text/html`, reproducing the decoder failure. | Multiple tenant-level issues, plus run duration. The health record reduces streaming failures to `streaming board failed with no progress`; only the journal preserves the actionable decoder failure. Preserve errors; investigate IAMGOLD's response and Zions' column layout before changing streaming or transport. |
| Teamtailor | Failed/truncated at 21:44: 1672 ingested, one failure, two interruptions; previous run had 19 failures. Logs show repeated 1000-page cap errors, gone boards, and expired certificates. | `thynk.cloud/jobs?page=1` and `page=2` both return 200, 67622-byte pages with the job-list container. | Some custom sites ignore pagination, spending up to 1000 requests before failing safely. Do not convert repetition into successful end-of-board proof. Parent can implement early repeated-page failure without weakening completeness. `careers.nobia.com` cert expired November 13 05:17 UTC, `jobb.frontwalker.se` November 13 06:47 UTC: employer certificate blockers. Do not disable TLS verification. |
| Trudvsem | Failed/truncated at 20:31: 137574 ingested, zero failed, five interruptions, 8997 skipped. Next run active. | Region 77, `limit=1&offset=0`, returns 200 JSON (4372 bytes). By 23:03 health includes a new actual success. | Crawl window exhaustion, not endpoint outage. Existing provider note explains six-second requests and oversized regions. Parent should evaluate scheduling/sharding coverage using completed-region counts, without raising request rate or running a mass crawl. |
| UKG | Failed/truncated at 20:45: 10166 ingested, zero failed, 79 interruptions. Journal explicitly records limiter waits exceeding the context deadline. | `abbank.rec.pro.ukg.net` tenant `and1501andb`, board `de567d88-f448-4cc3-bb54-cbb7e61e015c`, listing endpoint returns 200 with four opportunities and `totalCount=4` when sent the adapter's exact nested `opportunitySearch` body. | The adapter re-reads every opportunity detail each run under a shared ~5 req/s limiter. Parent should add known-ID hydration and then reassess coverage. Do not call the entire provider down based on unit failure; a unit fails on incompleteness too. |
| ADP | Only shard 2 failed at inspection. | Workforce Now listing for catalogue tenant `8a680274-0b2e-46cf-b2cc-49bd2ef0c20f:9201289910657_2` returns 200. | Shard 2 ingested 13470, failed zero, interrupted nine, truncated at 04:37. Other seven shards' latest runs ingested 11259–13033 each without truncation. A partial shard, not a provider-wide transport failure. Preserve interruption signal; assess shard-2 workload before rescheduling. |
| ADP MyJobs | All eight shard units failed; latest runs all truncated, 2–131 interruptions per shard. Shard 8 ingested 17522, zero failed, 104 interruptions. | Career-site metadata for `1138141confidential` returns 200 (3479 bytes). Journal shows `rate: Wait(n=1) would exceed context deadline`; one separate `rlghealthcarecareers` tenant returns 400. | Every run hydrates every posting, repeatedly consuming the shared request budget. Fixed with `HydratingSource`: enumerate as before, refresh known IDs without a detail call, hydrate new IDs normally. The pipeline's existing body-refresh policy remains responsible for edited bodies. Does not change rate or shard count. |

## Catalogue, scheduler, and status discrepancies

- `systemctl --failed` is a run-completeness signal as well as an outage signal.
  A deadline-truncated run exits nonzero even after ingesting many thousands of jobs.
  Successful cooldown-only runs can conversely show green while doing no work.
- Foxconn and RWB are scheduler-managed transient units named
  `freehire-ingest-run-foxconn.service` and `freehire-ingest-run-rwb.service`.
  Looking only at `freehire-ingest@foxconn.service` finds no journal. Scheduler
  launch records are under `freehire-ingest-scheduler.service`.
- The log's 21187 unhealthy and 8585 chronic counts span *all* health rows, including
  retired/pending/rejected/absent catalogue entries. `ListUnhealthyBoards` filters
  failures; `ListChronicBoards` filters last success/first seen older than 30 days.
  Neither restricts to active catalogue boards.
- A later case-insensitive catalogue join measured 8615 chronic health records:
  **1775 active, 130 pending, 85 rejected, 6291 retired, 334 absent**. Apploi alone
  accounts for 5833 stale health records and no scheduled active provider. This is
  retained history, not 5833 newly broken live boards. Counts move as workers run.
  Active chronic leaders: Personio 323, Greenhouse 201, BambooHR 150, Ashby 120,
  Paycom 113, Hireology 86, Teamtailor 78, Paylocity 68. Do not mass retire these.
- Join dominates the failure backlog: over 15000 health records with positive
  consecutive failures. It needs its own provider investigation; this batch did
  not own or modify Join.
- Public `ProviderHealthRollup` has a provider-level `EXISTS` active-board filter,
  not an identity-level join. Once a provider has one active board, *all* its health
  rows contribute, including retired or orphan records. Wholly retired Apploi is
  omitted, but mixed providers still have polluted denominators. Match by provider,
  case-insensitive board, and region if the parent changes this query.
- `last_success == generated_at` is intentional SQL:
  `max(CASE WHEN quiet_until > now() THEN now() ELSE last_success_at END)`.
  Migration 0198 introduced quiet pauses up to 72 hours, longer than status's
  48-hour freshness, so this synthesizes freshness for deliberately skipped boards.
  It is **not the time of an observed successful crawl**. A single quiet board can
  keep the provider fresh while others fail; healthy also means merely “not in
  active cooldown,” so expired cooldowns count healthy without another success.
  Example: HigherMe has 46 quiet rows, latest real success 14:20, then six 403s at
  22:23; the rollup still has synthetic current freshness. Parent should separate
  actual success from eligibility/freshness policy and expose incompleteness.

## Recovery plan for the parent

1. Review and release the three provider fixes through the normal deployment path.
   Allow natural cooldown expiry; do not clear failure history to improve status.
2. Verify Foxconn's next scheduled run refreshes known IDs, and Freshteam's
   `10times` detail is banked rather than unreadable. Sample other Freshteam templates
   before assuming all 3823 unreadables share one cause.
3. Verify ADP MyJobs completion and request reduction on one normal shard before
   altering its eight-shard schedule. Known-posting requests should cost listing
   calls only, with the existing periodic body refresh still active.
4. For RWB, spike only one listing through the existing challenge-capable service;
   accept a transport change only after receiving real vacancy JSON. For HigherMe,
   measure a paced bounded run below the refusal rate from its provider note.
5. Treat employer TLS renewal and CHECK24 public-API access as external blockers.
   For Taleo, UKG, SmartRecruiters and Trudvsem, use completed-board/region coverage
   and per-run interruption counts to choose targeted hydration or scheduling work.
6. Fix active-catalogue scoping and synthetic timestamp semantics in shared queries
   separately. No shared transport/pipeline/status changes were made here.
   Host configuration changes belong in `freehire-ops/provision/host2` and require
   explicit reviewed installation plus daemon reload; no host files changed here.

## Verification

- ADP MyJobs regression failed before implementation because it did not implement
  `HydratingSource`; passes afterward with exactly site + listing + one new detail.
- Foxconn regression failed before implementation with `none of 4 new postings
  could be read`; passes afterward without banking empty descriptions.
- Freshteam regression exercises the detail adapter using the observed DOM shape,
  verifies body/title/location, excludes the Liquid error, and leaves date unset.
- Existing targeted Foxconn, Freshteam and ADP MyJobs tests pass.
- A temporary replay test feeds the actual live `10times` HTML through `freshteam.detail` and checks title, body, location and exclusion of the Liquid error.
- Full checks and repository CodeRabbit review outcomes are appended after completion.

- IAMGOLD was re-probed with the adapter's complete POST payload (including both
  filter-selection objects), a cookie jar, portal, and timezone headers: HTTP 200
  `text/html` again. This confirms the failure is not an omitted probe field.
- First CodeRabbit review completed with two accepted findings: avoid a second
  entity decode on Freshteam's rendered HTML, and fail ADP MyJobs when every new
  detail is unreadable. Both are fixed with regression coverage. The ledger is
  untracked prose and was inspected manually; CodeRabbit reviewed the six Go files.
- Simplification pass retained the existing adapter interfaces and helpers; no
  transport wrapper, new dependency, scheduling configuration or data migration.

- Second CodeRabbit review completed (six Go files), raising two further points:
  retain sibling description sections in the Freshteam fallback, and carry ADP's
  listing title in known-ID refreshes. Both were addressed; regressions assert a
  sibling requirements paragraph and retained refresh title. No third review was
  run, following the repository skill's two-run limit.
- Final verification passes: repository-wide `gofmt -l .` (empty), `go vet ./...`,
  `go test ./...`, `go vet -tags=integration ./...`, targeted race tests for all
  three changed providers, `git diff --check`, and the repository file-size check.
  Whole-module commands used `GOMAXPROCS=2` and `-p 2` on the shared machine.
  An earlier unrestricted full run hit one-second timeouts in
  `TestRegister_DuplicateEmail` and `TestRegister_DBDown`; subsequent complete runs
  passed without changing auth code. No integration database suite was run.
