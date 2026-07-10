## 1. JustJoin adapter: detail hydration

- [x] 1.1 Add the detail types (`justJoinDetail` with `body`, `requiredSkills[].name`, `experienceLevel.value`) and a `hydrate` mapper that returns a Job with sanitized `body` → `Description`, skills via `skilltag.Parse(requiredSkills[].name)`, and seniority via `experienceLevel.value` (mid→middle, then `enrich.SeniorityValues` membership; empty when unmapped). Category is NOT set from justjoin (stack tag, title dict decides). Unit-test with a fake JSONGetter (routedHTTP): asserts description is sanitized and skills/seniority map.
- [x] 1.2 Add `FetchNew(ctx, e, seen func(externalID string) bool)`: page the list as `Fetch` does, hydrate only offers whose `guid` is not `seen` (bounded concurrency), mark seen offers `SeenRefresh` (liveness-only, no content), and isolate a single offer's detail failure (log + fall back to list-only). Unit-test: seen offer marked + no detail, unseen offer hydrates, one detail error does not abort.

## 1b. Seen-offer liveness refresh (must not wipe hydrated content)

- [x] 1b.1 Add `sources.Job.SeenRefresh` (mirroring `Removed`) and route it in the pipeline's buffered loop to a liveness-only `Touch` (via a `toucher` optional Store capability) instead of a content re-upsert. Add the `TouchJob` query (`last_seen_at`+reopen, no content), `make sqlc`, and `dbStore.Touch`. Unit-test the pipeline routing (seen → touch, new → save); integration-test `TouchJob` preserves description/skills while refreshing liveness + reopening.

## 2. HydratingSource optional interface

- [x] 2.1 Add `HydratingSource` interface (`Source` + `FetchNew`) to `internal/sources/source.go` and assert `justjoin` implements it. Unit-test: `NewJustJoin(nil).(HydratingSource)` type-asserts.

## 3. Pipeline seam: seen-set driven hydration

- [x] 3.1 Add the optional `seenLookup { ExistingExternalIDs(ctx, source) (map[string]struct{}, error) }` Store capability and wire `ingestBoard` to prefer `FetchNew` when the adapter is a `HydratingSource` and the Store is a `seenLookup`: load the provider's seen-set once, pass a membership predicate; fail open (empty set + log) on lookup error. Non-hydrating adapters and Stores without the capability use the existing `Fetch` path unchanged. Unit-test with fakes: hydrating adapter is driven by the seen-set; non-hydrating adapter untouched; lookup error → empty set, board still crawled.

## 4. Store: ExistingExternalIDs query

- [x] 4.1 Add the sqlc query `ExistingExternalIDs` (`SELECT external_id FROM jobs WHERE source = $1`), run `make sqlc`, and implement `seenLookup` on `cmd/ingest`'s `dbStore` (returning the set). Integration-test (build-tagged) that it returns the stored external ids for the provider.

## 5. One-time backfill command

- [x] 5.1 Add the sqlc queries the backfill needs (list `source='justjoin'` rows with id/url; update a row's description + refreshed content_hash), run `make sqlc`. Unit/integration-test the update query.
- [x] 5.2 Add `cmd/backfill-justjoin/main.go` (run-once via `worker.Main`): iterate justjoin rows, derive slug from the stored URL, fetch detail, update description, isolate + count per-row failures, log a summary. Manually smoke-run against a couple of live slugs.

## 6. Verification

- [x] 6.1 `go build ./... && go vet ./... && go test ./...` green; document the deploy step (run `cmd/backfill-justjoin` then `make reindex`) in the change notes.
