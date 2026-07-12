## 1. Tests (TDD — write first, red)

- [x] 1.1 In `internal/sources/reed_test.go`, add a test asserting `reed` implements
  `HydratingSource` and no longer implements `StreamingSource`.
- [x] 1.2 Add a `FetchNew` seen-set test (mirroring `justjoin_test.go`) over a fake HTTP
  client: given a search that unions ids where some are "seen" and some are new, assert
  seen ids yield a `SeenRefresh` job with NO detail request issued, and new ids are
  hydrated from their detail (description + employer `externalUrl`).
- [x] 1.3 Add a `Fetch` fallback test: with an always-false seen predicate (list-only
  path), every unique job's detail is fetched and no job is marked `SeenRefresh`.

## 2. Adapter change

- [x] 2.1 Remove `FetchStream` from `reed` (drop the `StreamingSource` marker); keep the
  `boardless()` and `aggregator()` markers.
- [x] 2.2 Add `FetchNew(ctx, e, seen)`: `searchIDs` then `fetchDetails(ids,
  reedDetailWorkers, fn)`, where `fn(id)` returns a minimal `Job{ExternalID:
  strconv(id), SeenRefresh: true}` when `seen(strconv(id))`, else `detail(ctx, id)`.
- [x] 2.3 Reshape `Fetch` to the list-only fallback: `FetchNew(ctx, e, func(string) bool
  { return false })`.
- [x] 2.4 Update the reed doc comment to describe the hydrating (detail-only-for-new)
  behavior and the per-hour-quota rationale; remove stale streaming references.

## 3. Verify

- [x] 3.1 `go build ./... && go vet ./...` and `go test ./internal/sources/...` pass.
- [x] 3.2 Confirm the pipeline path is unchanged (no edits under `internal/pipeline/`),
  and that `reed` is exercised via the existing `HydratingSource`/`seenLookup`/`touch`
  seam.
- [ ] 3.3 Sync the spec delta into `openspec/specs/source-ingest/spec.md` and archive the
  change per the project OpenSpec workflow.
