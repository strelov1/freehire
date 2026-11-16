# Ingest final recovery, 2026-11-15

Independent implementation scope: RWB, HigherMe admission, and the periodic body-refresh release prerequisite. Read both `a6ecaa687` and `c8a243647` versions of the fleet follow-up. No production configuration, cooldown, service, deployment or database mutation; no full crawl. Production reads and bounded public API requests ran from operator. The shared Scrapling routing file remains owned by the integration agent and was not edited here.

## Production configuration follow-up, 2026-11-16

The measured configuration is now published in
[freehire-ops PR 286](https://github.com/strelov1/freehire-ops/pull/286), merged as
`978127e5fabfe75c91ae011a7c91c7767ca181d2`. It was installed atomically on operator,
with backup, checksum, shell syntax, permissions and provider-scope assertions checked.
UKG uses `BODY_REFRESH_DAYS=30`, `BODY_REFRESH_SLICE=30`; ADP MyJobs uses days 30,
slices 90. Explicit overrides are preserved. The older local patch below proposed
30 slices for both and is superseded by that shared commit. Application deployment
and actual successful refreshes remain to be verified.

## RWB: browser route needs the pagination fix

Four direct-egress browser requests through the existing Scrapling service:

| Request | Upstream | Result |
| --- | --- | --- |
| `limit=200&offset=0` | 200 | 50 items, range count 233, echoed limit 200; 6.63s |
| `limit=200&offset=200` | 200 | 33 items, range count 233; 5.23s |
| vacancy 41107 | 200 | Description and 7 duties, 6 requirements, 6 conditions; 5.20s |
| `limit=50&offset=50` | 200 | 50 items, no overlap with offset 0 |

The server silently caps actual items at 50 despite echoing 200. The original adapter skipped offsets 50–199. `TestRWBListingUsesFiftyItemPages` failed on the original URL (`limit=200&offset=0`) and passes with the page size fixed to 50. Existing multi-page mapping coverage was adjusted to that page size.

A temporary local Go replay used the exact `rwb.list`, `rwbItem` and `rwb.detail` parsers over captured responses. It parsed offsets 0 and 50 and then deliberately failed at offset 100, bounding the replay. Detail 41107 mapped to an identified Moscow role with title, location, public URL and 3,584-byte sanitized body. This replay passed and its temporary test was removed. No synthetic body substituted for the captured detail. Live captures remain under `/tmp/rwb-*.json` locally, not in the repository.

The earlier HTTP/direct result was upstream 498. The independently validated browser route is in `2026-11-15-ingest-rwb-route.patch` alongside this report for the integration owner to apply to `scraplingtier.go`. Apply it together with the pagination fix using `git apply --unidiff-zero` (or insert the four-line entry beside the map declaration). The context-free artifact avoids embedding tabbed context as whitespace errors in the documentation diff. It uses the existing browser client and its timeout, retry policy and status/error preservation. RWB's existing detail-failure skip remains; it does not claim full-board completeness. These samples do not establish sustained browser capacity.

## HigherMe: retries now share admission

`pacedHigherMeGetter` previously admitted a logical getter call, then `Client.do` could retry without admission. Production now clones only the HigherMe client and wraps its transport so every attempt, including retries, redirects and any configured refusal lane, draws from one limiter shared by its boards. Other providers' shared client is untouched. Admission waits outside the original per-attempt HTTP timeout, while the inner client retains that timeout; redirects return to the outer paced transport. This prevents queued boards timing out before sending. A behavior test holds admission longer than the wire timeout and still receives a valid JSON response. Cancellation returns the gate error and prevents the wire request. The three-second interval is unchanged: it is not a validated safe sustained rate. Around 120 initial requests fit six minutes even before retries, beyond the earlier roughly 70-request refusal observation.

The regression drives the real `Client` against a local HTTP server: one 503 and its retry, then another board request, require three admissions. It first failed to compile without the new helper, then passed with the production helper. Status handling, retry count, backoff and response decoding remain the client's existing implementation.

The bounded operator experiment requests only page 1, limit 1, of fixture brand `6078977d8dfec` at 30-second spacing, maximum 13 requests, no retries, stop on any non-200. This interval is an experimental input, not a replacement production limit. Every response is checked for a real one-item listing (reported total 7); the operator process table is sampled before each request for concurrent HigherMe ingest. An initial request to a mistaken brand returned an empty 200 and was excluded; that probe was stopped, not treated as validation. Its traffic still counts as one extra request before the valid experiment.

Only HigherMe's adapter and registry target `api.higherme.com` in the source tree. No other provider was found sharing this origin. Separate tools/processes are not coordinated by the in-process bucket, and process-table sampling cannot prove all traffic from an IP. The valid experiment completed 13/13 upstream 200s in 360.82 seconds, each with one real posting and reported total 7, zero retries and zero concurrent HigherMe ingest processes observed. Including the excluded empty-brand probe, the experiment made 14 public API requests. Raw per-attempt accounting is in `/tmp/higherme-probe-valid.jsonl`. No throughput claim or fleet-wide safe limit follows from a short low-volume sample. Production sustained pacing remains a blocker requiring an aggregate-origin observation over real traffic, without resetting cooldowns or launching a full crawl for this task.

## Persistent body refresh: prepared ops change, activation gated

Read `freehire-ops/provision/host2/AGENTS.md`, its ingest units and wrapper. Operator's UKG template and ADPMyJobs eight-shard template use `/opt/freehire/.env` and `/opt/freehire/bin/ingest-slot.sh`. Both body-refresh variables remain absent in that environment file. `sweep.BodyRefreshFor` disables periodic refresh when days is absent. The newly hydrating UKG and ADPMyJobs implementations therefore require an ops activation before periodic employer edits can be claimed to refresh.

The reviewable patch is in the separate freehire-ops checkout:

`/Users/i_strelov/Projects/freehire/freehire-ops/artifacts/2026-11-15-body-refresh.patch`

It changes only `scripts/host2/ingest-slot.sh`, after provider extraction: UKG and ADPMyJobs default to `BODY_REFRESH_DAYS=30`, `BODY_REFRESH_SLICE=30`, preserving explicit nonempty overrides. Other providers inherit their existing environment unchanged. The wrapper covers static units, all ADPMyJobs shards and scheduler-launched runs. No shared `.env` change enables the rest of the fleet. The dirty `freehire-reindexw.service` was left untouched; the ops patch itself is an uncommitted artifact, not an installed change.

Thirty slices is the implementation's existing default, not a hard request budget: rows hash by external ID, and today's slice is `YearDay()%30`. All runs on the same date reuse the slice; successful hydration removes a row until its body ages again, while failed hydration can be retried on later runs that day. Stored closed rows are also eligible in the seen-set query, although only rows re-listed by the source incur hydration. An uninterrupted daily cycle normally adds up to 29 days after the age cutoff; the day-of-year reset can lengthen the boundary gap. Uneven hashes, newly discovered jobs and failures can add requests. Neither adapter currently caps new hydration per run. UKG's pacing is shared across details and listing calls, while ADPMyJobs runs eight independent shards; adapter concurrency is not an origin-wide aggregate rate guarantee.

Pre-activation budget measurement attempted a read-only `jobs`/`job_activity` grouped count of stale stored/open rows and their largest 30-way slice, with a 25-second statement timeout. The query timed out; there is no verified slice size or run-time headroom in this report. An initial count query referenced a nonexistent column and was corrected to `closed_at IS NULL`; neither yielded a measurement. Do not represent activation as safe or complete until that budget is measured, per shard/board and remaining run deadline, including retries and normal new postings. Increase slicing or defer activation if the largest refresh slice cannot fit. The existing daily slice is a distribution mechanism, not a cap on attempts.

Installation plan for the authorized operator, after the integration binary and budget prerequisites pass:

1. Review/apply the patch in freehire-ops, confirm no concurrent edits to the wrapper, and commit that repository change. `git apply --check` passes against the current checkout.
2. Recheck the production wrapper against that reviewed source and account for any drift. Back up the installed script, then install the reviewed wrapper atomically at `/opt/freehire/bin/ingest-slot.sh` with its existing owner/mode. This task does not install it. No unit change or daemon reload is needed for a script-only replacement.
3. Let the ordinary schedule exercise the new binary and wrapper. Observe per-run requested details, unreadable/failed counts, elapsed time, deadline exits and upstream refusals for both providers and every ADPMyJobs shard. Confirm stored `hydrated_at` advances on body reads and edited bodies update. Keep other providers disabled.
4. Roll back by restoring the saved wrapper if the measured refresh cost does not fit; restore original environment semantics rather than resetting cooldowns. Deploying an app binary alone does not install this ops change.

The candidate wrapper passes `bash -n`, ShellCheck and an assertion-based check for targeted defaults, explicit overrides and unchanged unrelated-provider values. Budget validation and production installation remain explicit release prerequisites.

## Verification and review

Required checks, simplification and repository review results are appended after completion. Initial local attempts under disk pressure do not count as passes. Disposable compiler cache older than one hour was reclaimed to make room; no source or production data was removed.

CodeRabbit's first review completed over all nine intended files, with two accepted findings. Admission inside the attempt timeout had already been corrected during local review. The second identified a host hold recorded by another worker while admission was waiting: the transport now rechecks direct-host backoff after admission and before transmission. Refusal egress remains exempt. A regression injects a hold during admission, expires the caller context and requires zero wire requests. The final revision is reviewed again below. Simplification retained the provider-specific client seam and existing client retry/status handling; no shared transport or other adapter behavior was rewritten.

Final required checks pass on the reviewed Go revision, with `GOMAXPROCS=2` and `-p 2`: `go vet ./...`, `go test ./...`, and `go vet -tags=integration ./...`. Targeted source/sweep tests and HigherMe's four new regressions under `-race` also pass. Repository `gofmt -l .` is empty, file-size and staged whitespace checks pass. The route artifact passes `git apply --check --unidiff-zero`; the separate ops artifact passes its apply check, ShellCheck, syntax and scope assertions. The full tagged integration suite was not run; no database code changed. Final check logs use `/tmp/ingest-recovery-reviewed-*`, and race verification completed successfully on the four HigherMe regressions.

The second CodeRabbit review completed over all nine files. Two findings asserted absent companion changes (RWB's production page-size constant and the registry's real-client branch); both are already in this diff and their tests pass. A third finding correctly identified that admissions spent before a host hold expires can release a burst afterward. The transport now waits out a hold and obtains fresh admission before sending. A concurrent real-limiter regression lets the hold expire successfully and verifies separated wire requests; a production-constructor regression also pins the real-client branch. No third CodeRabbit invocation was made (the skill caps the workflow at two); the final small correction is checked locally with the full required suite and targeted race tests. Latest final logs use `/tmp/ingest-recovery-complete-*`.

The post-hold spacing regression was also checked red with a Go overlay restoring the prior single-admission implementation, without modifying the worktree used by full checks: it failed with a 25-microsecond post-hold burst. The corrected implementation passes, including under the race detector. The final source keeps the original production interval; sustained HigherMe recovery still requires measured aggregate-origin pacing, rather than extrapolating the 13-request probe.
