## 1. Working Nomads adapter (pattern-locker, build first)

- [x] 1.1 Wrote table-driven `workingnomads_test.go` on the `routedHTTP` fake (live fixture inlined per the package convention): asserts per-job company from the payload, `ExternalID` parsed from the `/job/go/<id>/` URL, drop of a no-id URL, `Title`/`Description`(sanitized)/`Location`/`PostedAt` mapping, `Remote`+`WorkMode="remote"` (remote-only board), and the `boardless`+`aggregator` markers (RED ✓ — undefined NewWorkingNomads)
- [x] 1.2 Implemented `internal/sources/workingnomads.go`: `NewWorkingNomads`, `Provider()`, `boardless()`, `aggregator()`, `Fetch` single-request decode of the flat array, posting→`Job` with `workingNomadsIDRE` id-from-URL (GREEN ✓ — 5/5 tests pass, build+vet+package tests green)

## 2. Himalayas adapter

- [x] 2.1 Wrote `himalayas_test.go`: asserts offset pagination across `totalCount` (totalCount 150 > limit 100 → exactly 2 requests via `fake.calls`), `Company=companyName`, `ExternalID=guid` (empty-guid dropped), `URL=applicationLink`, `Location` joined from `locationRestrictions`, epoch-seconds `pubDate`, `Remote`+`WorkMode="remote"` (RED ✓)
- [x] 2.2 Implemented `internal/sources/himalayas.go`: offset/limit loop driven by `totalCount` under `himalayasMaxPages` cap, `parseEpochSeconds(pubDate)`, `joinNonEmpty(locationRestrictions...)` (GREEN ✓ — 5/5 pass)

## 3. Landing.jobs adapter — DROPPED

- [x] 3.1 DROPPED during implementation: the `/api/v1/jobs` list response has **no structured
  company field** (employer only as a URL slug `/at/<slug>/` and in description prose).
  Deriving company from a concatenated slug would pollute the companies table — an MVP-shortcut
  the project forbids. Moved to design's rejected alternatives. No `landingjobs.go` written.
- [x] 3.2 DROPPED (see 3.1)

## 4. Remotive adapter

- [x] 4.1 Wrote `remotive_test.go`: asserts a **single** fetch (`fake.calls==1`), `Remote`+`WorkMode="remote"`, `Company=company_name`, `ExternalID=id` (zero-id dropped), zoneless-ISO date mapping (RED ✓)
- [x] 4.2 Implemented `internal/sources/remotive.go`: single `GetJSON`, dedicated `remotivePubLayout="2006-01-02T15:04:05"` (RFC3339 would reject Remotive's zoneless timestamp — confirmed live) (GREEN ✓)

## 5. JustJoin adapter

- [x] 5.1 Wrote `justjoin_test.go`: asserts cursor pagination (2 pages via `fake.calls==2`), synthesized `URL=https://justjoin.it/job-offer/<slug>`, `WorkMode` from `workplaceType` (incl. `office`→onsite), `Company=companyName`, `ExternalID=guid` (empty-slug dropped), RFC3339-millis date (RED ✓)
- [x] 5.2 Implemented `internal/sources/justjoin.go`: cursor loop with strictly-increasing guard + `justJoinMaxPages` cap; **live-smoke corrected the pagination param `?cursor=`→`?from=`** (`?cursor=` is silently ignored — caught by live curl, the guard meant the wrong param degraded to 20 jobs, never an infinite loop); added `office` to the shared `workplaceTypeMode` (GREEN ✓)

## 6. Wiring

- [x] 6.1 Registered all four `New…(c)` in `sources.All` (`internal/sources/source.go`), boardless-aggregator block
- [x] 6.2 Added `sources/{workingnomads,himalayas,remotive,justjoin}.yml`, each one boardless entry
- [x] 6.3 Ran `make gen-contracts`; confirmed all four providers in `SOURCE_VALUES` (`landingjobs` correctly absent)

## 7. Verify

- [x] 7.1 `go build ./...`, `go vet ./...`, full `go test ./...` green; `gofmt` clean
- [x] 7.2 Live smoke: single-fetch adapters run end-to-end against the real APIs via a throwaway gated test (since deleted) — **workingnomads 46 jobs**, **remotive 30 jobs**, each with correct per-job company/id/url/`WorkMode="remote"`/date. JustJoin `?from=` pagination advance verified by live curl (page1 guid ≠ page2 guid). Himalayas decode verified against the live-captured fixture. Full-catalogue paginating crawl deferred to the deploy-time ingest (would fetch tens of thousands).
- [x] 7.3 Follow-up (for PR description): per-provider daily cron in `freehire-ops` (one schedule per `sources/<provider>.yml`) + a `make reindex` after first ingest, both out of this repo. Respect Remotive's documented ~4 req/day limit when scheduling.
