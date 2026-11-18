# Ingest fleet follow-up, 2026-11-15

Continues investigation commit `4a6065ebca869fc7e7425bab2735e62d9f9622ed`
without importing it. Production inspection used operator read-only. No production
writes, deployment, cooldown resets, full crawls, or host changes were made.

## Changes and semantics

- UKG `FetchNew` enumerates the same listing and preserves identity/title/location/
  date/work mode, emitting known IDs as `SeenRefresh` without detail requests.
  The existing store removes body-refresh-due IDs from its seen set, so those IDs
  hydrate again. Partial detail failures emit `Unreadable`; wholly unreadable
  requested details fail even with known refreshes. Failed refreshes cannot replace
  full bodies with briefs. Ordinary `Fetch` retains its existing brief fallback.
- Teamtailor fails at the third occurrence of a non-empty listing link set,
  regardless of ordering. One duplicate-only page is tolerated for overlap.
  Failure returns no partial result or false completeness. The 1000-page ceiling
  remains for endless distinct pages.
- Unhealthy/chronic reports now include scheduled active/pending catalog identities
  only. The public rollup keeps its existing active-only scope, now per identity.
  All three match provider, case-insensitive board, and exact region via `EXISTS`,
  avoiding duplicate counts. Excluded health history remains intact.
- Public `last_success` is the actual maximum stored success. Internal
  `freshness_at` supports legitimate quiet pauses beyond 48 hours, requiring a
  real success and zero subsequent failures. Family folding carries both fields.
  No HTTP contract shape change or TypeSpec/ogen edits; sqlc regenerated.
  Healthy still means outside active cooldown, not failure-free. Existing maximum
  freshness aggregation remains: one success does not prove every board complete.

## Bounded read-only production evidence

RWB's listing `limit=1&offset=0` through existing Scrapling, direct egress:
HTTP mode returned upstream 498, 1144 bytes, non-JSON in 0.2 seconds. Browser mode
returned upstream 200, 408 bytes, JSON in 6.9 seconds. A second browser request
verified a real vacancy: `status=200`, `data.items[0].id=41107`, a named logistics
role in Moscow; `data.range={count:233,limit:1,offset:0}`. This proves a real
listing lane, not detail hydration, pagination reliability, or throughput. Shared
transport configuration was outside this task's ownership; no egress switch.

HigherMe's journal shows six page-one 403s three seconds apart, 22:23:44–22:23:59
UTC, after run start 22:22:05: zero ingested, six failed, three cooled, 46 quiet.
No HigherMe process was running in the later snapshot. Registry construction
shares a three-second/burst-one limiter per process. Retries inside the getter
are outside that admission boundary. The documented ~70 requests/six minutes is
an observed refusal point, not safe capacity. Three-second admission allows
roughly 120 initial requests in that window, before retries. The journal cannot
establish aggregate IP traffic or a safe replacement pace. No unproven rate change
or common-transport edit: a bounded paced experiment recording retries and other
processes sharing the IP remains necessary.

## Verification and remaining limits

Regressions cover known-ID request counts, body rehydration, partial/all unreadable
UKG details, three-request Teamtailor failure and single-overlap traversal, SQL
active/pending/history scope, case/region identity, cap/full totals, retained
failure evidence, and quiet freshness with honest success timestamps.

Initial required-check attempts hit local disk exhaustion. Disposable compiler
cache and abandoned build artifacts were reclaimed; those failed runs do not
count as verification. Final results and review disposition follow below.

Simplification retained the existing hydration/marker interfaces, combined the
post-fetch counters into one pass, and reused UKG's listing mapping. CodeRabbit's
first review completed over 11 tracked files, raising one question: could
`SeenRefresh` clear the stored body/employment type? Rejected after tracing
`pipeline_write.go`'s refresh branch through `touchSeen` and the store's `Touch`:
refreshes touch liveness instead of upserting content. Existing pipeline behavior
and tests deliberately enforce that contract. Untracked SQL regressions and the
ledger were not included in the first review; a second pass includes staged files.

The pre-existing quiet-board integration regression now asserts internal policy
freshness and separately requires the public rollup's real success to stay 60h
old. `ProviderIngestHealth` and its Prometheus freshness semantics are unchanged;
this work does not rename or redefine that metric. Source-page health uses the
same separate policy field as status, while both wire surfaces report real
success. A regression also folds policy freshness independently across a family.

Second CodeRabbit review completed over 16 files, including the new SQL cases.
Both findings referred to potentially absent companion changes: SQL source for
the generated query, and `providerRollup.freshnessAt` for the source-page caller.
Both companions are present in this diff. Regenerating sqlc preserves the generated
files byte-for-byte, and handler tests compile and pass. No fixes were warranted.
The later adjustment to the existing quiet-board regression was inspected manually
and executed against Postgres; it was outside the second review's file list.

Final verification passes with `GOMAXPROCS=2` and `-p 2`: full `go vet ./...`,
full `go test ./...`, and `go vet -tags=integration ./...`. Actual Postgres tests
pass for `TestBoardHealthCatalogScope`, `TestProviderHealthQuietFreshnessKeepsActualSuccess`,
`TestListChronicBoards*`, `TestQuietBoardsCountAsFresh`, and handler
`TestIngestStatusEndpoint`. Adapter/handler suites and the final policy/family
regressions pass. Repository `gofmt -l .` is empty, sqlc regeneration is stable,
file-size checking and staged diff whitespace checks pass. The full tagged suite
was not run; the changed SQL and status endpoint were exercised directly.

## Deployment follow-up: body refresh is opt-in

A final read-only operator check found `BODY_REFRESH_DAYS` and
`BODY_REFRESH_SLICE` absent from `/opt/freehire/.env`. The effective UKG unit
uses that environment file and `ingest-slot.sh`; no body-refresh override was
found in the unit files or wrapper directory. `sweep.BodyRefreshFor` disables
periodic body refresh when the days setting is empty. The adapter preserves that
existing mechanism, but enabling it for production remains an explicit ops
follow-up before claiming edited UKG bodies will refresh periodically. No host
configuration was changed. The integration agent should coordinate this with
other newly hydrating adapters and the existing sliced refresh budget.

Implementation commit: `c8a243647ce48cb791ccdb5e72b180825cf0ba33`. All pre-commit
checks passed, including lint, sqlc drift, go vet, gitleaks, and doc links.
The commit-message hook skipped commitlint because root pnpm dependencies were
not installed; the title uses the repository's required Conventional Commit form.
