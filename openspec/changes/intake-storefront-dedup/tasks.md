## 1. The catalogue lookup

- [ ] 1.1 Write the failing integration test for `CanonicalJobForRole`: it finds the open
      canonical posting of a role cluster, excludes the row being imported by its own
      `(source, external_id)`, and returns no rows when every candidate is closed or is itself a
      duplicate (`internal/db/canonical_job_for_role_integration_test.go`, package `db`)
- [ ] 1.2 Write the failing integration test for `MarkJobDuplicateOf`: it points one row at its
      canon and reports one row affected
- [ ] 1.3 Add both queries to `internal/db/queries/jobs.sql`, run `make sqlc`, and make the
      tests pass

## 2. The collapse on import

- [ ] 2.1 Write the failing integration test in `internal/linkimport`: with a crawled posting
      seeded in the same role cluster, importing a storefront page writes a row carrying
      `duplicate_of`, returns the canonical `public_slug`, and queues no enrichment for it
- [ ] 2.2 Add `Deduped` to `linkimport.Result` and the canon lookup to `write`, gated to the
      generic identity and to a non-empty `role_fingerprint`; on a hit mark the row, skip the
      enrichment enqueue, and skip the search push
- [ ] 2.3 Verify the test catches the regression: stub the lookup to find nothing, confirm the
      test fails, restore
- [ ] 2.4 Confirm the pre-existing `linkimport` tests still pass — an import with no canon must
      still be enqueued and indexed

## 3. The intake answer

- [ ] 3.1 Write the failing integration test in `internal/handler`: a storefront link whose
      vacancy the catalogue carries answers 200 `found` with the canonical slug, and the
      contribution row for its board exists after the call
- [ ] 3.2 Answer `found` for a deduped import in `intakeService.Resolve`, after the contribution
      is recorded, leaving the early catalogue-hit `found` untouched
- [ ] 3.3 Confirm every pre-existing `TestResolveJobEndpoint` subtest is still green

## 4. Documentation

- [ ] 4.1 Restate the outcome in `internal/contribution/AGENTS.md`: `found` is reachable twice —
      before any fetch, and after an import that collapsed onto an existing posting
- [ ] 4.2 Update the `/jobs/resolve` description in `web/src/lib/docs/api-spec.ts` and
      regenerate `docs/API.md` (`cd web && pnpm gen:api-docs`)

## 5. Verification

- [ ] 5.1 `go build ./... && go vet ./... && gofmt -l internal cmd` clean, `go test ./...` green
- [ ] 5.2 `go test -tags=integration ./internal/db/ ./internal/linkimport/ ./internal/handler/
      ./internal/contribution/` green
- [ ] 5.3 `cd web && pnpm lint && pnpm build` — lint reports 0 errors
