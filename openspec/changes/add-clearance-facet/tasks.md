## 1. The dictionary

- [x] 1.1 Write the failing table test for the anchored-phrase matcher in
  `internal/dict/location/clearance_test.go`, covering one case per spec scenario:
  UK/US/AU anchors fire, bare `SC`/`DV` do not, silence yields unknown.
- [x] 1.2 Implement `RequiresClearanceFromDescription` in
  `internal/dict/location/clearance.go` — the phrase list and word-boundary walk,
  reusing the unexported matcher primitives already in `eligibility.go`.
- [x] 1.3 Extend the test with the negation scenarios (`no security clearance
  required`, a denial elsewhere in a description that also names a clearance), then
  wire the existing negation guard in so they pass.
- [x] 1.4 Extend the test with the unrelated-sense scenarios (`customs clearance`,
  `medical clearance`, `clearance sale`) and confirm they do not fire.
- [x] 1.5 Extend the test with the obtainable-clearance scenarios (`must be able
  to obtain a security clearance`, `eligible for SC clearance`) and make them mark.
- [x] 1.6 Write the failing test for the labelled-field rule (`Clearance: Secret`,
  `CLEARANCE REQUIRED FOR START: Yes`, `Clearance Level: Public Trust` mark;
  `Clearance Required: No`, `Clearance: None`, `Clearance: N/A` do not), then
  implement it.

## 2. Derivation and storage

- [x] 2.1 Add migration `0119_jobs_requires_clearance.sql` — `ALTER TABLE jobs ADD
  COLUMN requires_clearance boolean`, with a comment stating the tri-state meaning,
  modelled on `0017_jobs_is_tech.sql`.
- [x] 2.2 Add `RequiresClearance *bool` to `jobderive.Derived`, test-first, and
  populate it from the dictionary in `Derive`.
- [x] 2.3 Thread the field through the `job` aggregate (`internal/job/job/job.go`)
  — the struct field and all four persistence shapes — following `IsTech`
  line for line.
- [x] 2.4 Add the column to the upsert/update statements in
  `internal/platform/db/queries/jobs.sql` everywhere `is_tech` appears, then run
  `make sqlc`.
- [x] 2.5 Verify all three write paths (ingest, moderator authoring, Telegram)
  derive it, per the spec scenario that they cannot diverge.

## 3. Serving and filtering

- [x] 3.1 Serve `requires_clearance` from `internal/job/jobview`, omitted when
  unknown, test-first — mirroring how `is_tech` renders its tri-state.
- [x] 3.2 Add `requires_clearance` to the Meilisearch filterable set in
  `internal/search/search/client.go`.
- [x] 3.3 Map the query parameter in `internal/search/search/query_filter.go`,
  test-first — including that `false` yields `IS NULL OR = false`, not a plain
  equality, so the unknowns are returned.
- [x] 3.4 Confirm an absent parameter changes no behaviour (spec scenario).

## 4. Documentation and UI

- [x] 4.1 Document the parameter in `web/static/openapi.yaml` and
  `web/src/lib/docs/filters.ts`.
- [x] 4.2 Add the facet to the web filter model and URL encoding
  (`web/src/lib/facetModel.ts`, `web/src/lib/types.ts`).
- [x] 4.3 Add the "Hide jobs requiring security clearance" checkbox to the filter
  panel.

## 5. Backfill

- [x] 5.1 Write the backfill as a one-off `cmd/` worker: name candidates via
  Meilisearch (the `clearance` token plus `ts/sci`, `polygraph`, `bpss`,
  `vetting`, `agsva`), re-derive only those rows, `IS DISTINCT FROM`-guarded so a
  re-run writes nothing.
- [x] 5.2 Test that it is idempotent and that it reads only candidate rows.

## 6. Verification and rollout

- [x] 6.1 `gofmt -l .` clean, `go vet ./...`, `go test ./...`, and
  `go vet -tags=integration ./...` all pass.
- [x] 6.2 Sample the matcher against real prod descriptions and record the
  measured precision — no false positive among the inspected marked rows.
- [ ] 6.3 Apply migration 0119 on prod.
- [ ] 6.4 Patch the live Meilisearch settings to declare the attribute, and read
  the settings back to confirm — **before** deploying the binary.
- [ ] 6.5 Deploy, then verify `/api/v1/jobs/facets` still answers 200 and
  `?requires_clearance=false` filters.
- [ ] 6.6 Run the backfill and record the true marked-posting count.
- [ ] 6.7 Run a full Meilisearch rebuild. The incremental push only sends documents
  whose `content_hash` moved, and the new column is not in that hash — the trap
  `is_tech` already fell into — so without this the facet is empty for every
  pre-existing posting. Pause `freehire-reindexw.timer` first.
