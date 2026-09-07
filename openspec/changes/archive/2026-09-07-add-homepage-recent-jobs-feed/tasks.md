## 1. Database

- [x] 1.1 Add migration creating `recent_feed_outbox (job_id bigint PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE, created_at timestamptz NOT NULL DEFAULT now())`
- [x] 1.2 Add sqlc query to enqueue a job id (`ON CONFLICT DO NOTHING`, matching `EnqueueSearchOutbox`'s shape)
- [x] 1.3 Add sqlc query to claim-and-delete a bounded batch (`SELECT ... FOR UPDATE SKIP LOCKED` joined to `jobs` for id, title, company, public_slug — no separate `companies` join needed, `jobs.company` already carries the denormalized display name), then run `make sqlc`

## 2. Ingest write path

- [x] 2.1 In `cmd/ingest/store.go`, add the outbox enqueue call next to the existing `EnqueueSearchOutbox` call, gated on the same `needsIndex(saved) && !deduped && !saved.duplicateOf.Valid` condition plus the existing IT/tech check
- [x] 2.2 Unit test: eligible canonical IT job enqueues; duplicate/repost does not; non-IT job does not

## 3. `internal/job/recentfeed` package

- [x] 3.1 Register the new package in `internal/platform/arch/layering/blocks.go` under the `job` block
- [x] 3.1a Export `NormalizedRoleTitle(title string) string` from `internal/job/jobhash`, extracted from `RoleFingerprint`'s existing tag-strip/entity-decode/lowercase/whitespace-collapse/trailing-clause-removal logic, applied to the title alone (no company slug, no description); unit test it directly (existing `RoleFingerprint` tests are the source of truth for expected normalization behavior)
- [x] 3.2 Implement grouping: given a batch of claimed rows, group by `jobhash.NormalizedRoleTitle(title)` and produce single-job events (group size 1) or aggregated events (group size >= threshold constant)
- [x] 3.3 Unit tests for grouping: single job produces a `single` event; a below-threshold cluster produces one `single` event per job; an at/above-threshold cluster produces one `aggregate` event with the correct count and a representative job
- [x] 3.4 Implement the `Broadcaster`: fixed-size ring buffer (~15 entries) plus fan-out to subscriber channels; unit test covering backlog replay on a new subscription and delivery to multiple concurrent subscribers
- [x] 3.5 Implement the `Poller`: ticks every ~10s, claims a batch, runs grouping, publishes events to the `Broadcaster`; integration-tagged test against the real outbox table (claim removes rows; a stalled/empty outbox produces no events)

## 4. API endpoint

- [x] 4.1 Add `GET /api/v1/feed/recent` in `internal/api/handler` (new file), reusing `sseHeaders`/keepalive/`recoverStream` from the existing SSE machinery; on connect, replay the `Broadcaster`'s ring buffer then stream new events
- [x] 4.2 Route it as public (no auth middleware)
- [x] 4.3 Handler test (no DB needed, so plain `_test.go` rather than integration-tagged): a real TCP listener + streaming HTTP client confirms a connecting client receives backlog immediately, then receives a newly published event; a nil `Broadcaster` degrades to 503

## 5. Wiring

- [x] 5.1 In `cmd/server/main.go`, construct the `Broadcaster`, start the `Poller` goroutine, and pass the `Broadcaster` into the new handler

## 6. Frontend

- [x] 6.1 Add `web/src/lib/components/RecentJobsFeed.svelte`: opens an `EventSource` on `/api/v1/feed/recent`, keeps the last ~8-10 entries, animates new ones in
- [x] 6.2 Render each entry: role title + company logo via the existing `EntityLogo` primitive and `companyLogoUrl(name)` (same pattern as `JobRow.svelte`; no new placeholder logic — `EntityLogo` already falls back to a monogram); aggregated entries read as "role — +N more at other companies" and link to `/jobs`
- [x] 6.3 Render nothing when no entries have arrived yet (no empty-state placeholder)
- [x] 6.4 Wire the component into `web/src/lib/components/HomeLandingView.svelte` (the homepage's actual content, rendered by `+page.svelte`), right after the "catalogue, today" figures section and before "take it with you" — high on the page, just below the hero, following the existing `border-t` section rhythm
- [x] 6.5 Frontend unit test: the pure list/label logic (`recentFeed.ts` — push/cap/order, aggregate wording) is unit-tested directly; no `@testing-library/svelte` dependency exists in `web/` to render the component itself, so the component stays a thin consumer of that tested logic (mirrors `matchAnalysis.ts`/`MatchAnalysisFull.svelte`'s existing split)

## 7. Verification

- [x] 7.1 `go build ./... && go vet ./...`, `go test ./...`, `go vet -tags=integration ./...` all clean; full `go test -tags=integration` green for every touched package (`cmd/ingest`, all of `internal/job/...`, `internal/platform/arch/layering`, `internal/platform/db`) plus `gofmt -l .` clean and `pnpm check:sql` clean on the new migration
- [x] 7.2 Manual check with a fully isolated Postgres+Redis (not the shared dev stack, to avoid touching another concurrent task's data): ran the real `cmd/server` binary, inserted rows simulating an ingest write directly, confirmed over curl that a single posting produces a `single` SSE event, a 5-job burst across different companies produces one `aggregate` event, a new connection replays the backlog instantly, and the outbox row is deleted after claim. Then ran the actual SvelteKit dev server against it and drove a real headless Chromium (Playwright) to the homepage: the "just added" section renders both cards with correct copy and logos in the right place on the page (screenshot confirmed). The one console error observed (`503` on `/api/v1/jobs/facets`) is unrelated — Meilisearch was not part of this narrow verification stack.
- [x] 7.3 `openspec validate --change add-homepage-recent-jobs-feed --strict`
