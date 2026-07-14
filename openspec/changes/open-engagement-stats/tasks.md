## 1. Backend — engagement endpoint (engagement-stats)

- [x] 1.1 Add a `GetEngagementStats :one` query to `internal/db/queries/stats.sql` — `count(*) FILTER (WHERE saved_at/applied_at/viewed_at IS NOT NULL)::int` over `user_jobs`; run `make sqlc` and commit the generated code
- [x] 1.2 Add an `EngagementStats` handler in `internal/handler/stats.go` (a `UserGrowth` sibling) returning `{"data": {"saved","applied","viewed"}}`; aggregate-only, no PII
- [x] 1.3 Wire `GET /stats/engagement` in `handler.Register` as an unauthenticated public read next to `/stats/user-growth`
- [x] 1.4 Integration test: seeded saves/applies/views produce the expected counts; empty table → all zeros; public (no auth) 200

## 2. Frontend — API client + /open section

- [x] 2.1 Add an `EngagementStats` type and `api.engagementStats()` in `web/src/lib/api.ts` / types, mirroring `userGrowth`
- [x] 2.2 Add an `engagement` best-effort leg to `/open/+page.server.ts`'s `Promise.allSettled` fan-out
- [x] 2.3 Add an "Engagement" stat-strip section to `/open/+page.svelte` (jobs saved · applications · jobs viewed), each figure linking to `/api/v1/stats/engagement`; render a fallback when the slice is null

## 3. Verification

- [x] 3.1 `go build ./... && go vet ./... && go test ./...` and `npm run check` pass; load `/open` against the live API and confirm the Engagement section renders (and degrades when the endpoint is unreachable)
