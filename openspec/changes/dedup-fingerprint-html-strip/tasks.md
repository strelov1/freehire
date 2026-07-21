## 1. Fingerprint normalization

- [x] 1.1 Add failing tests to `internal/jobhash/rolefingerprint_test.go`: (a) two params with the same company + stripped title + same visible description but markup-only differences (extra `<br>`, `&amp;` vs `&`) produce the SAME fingerprint; (b) a real visible-text difference (a city-specific clause present in one, absent in the other) still produces DIFFERENT fingerprints; (c) an entity-encoded title (`R&amp;D …`) matches its decoded form.
- [x] 1.2 Implement the HTML→visible-text step in `normalizeRoleText` (strip tags via a package-level `<[^>]*>` regex, replacing each tag with a space, then `html.UnescapeString`, then the existing lowercase + `strings.Fields` fold). Strip tags before unescaping. Update the doc comment to state the fingerprint compares visible text.
- [x] 1.3 Run `go test ./internal/jobhash/...` — new tests pass AND all existing `TestRoleFingerprint_*` (case/whitespace/city-suffix/seniority/two-word-guard/field-delimiter) stay green.
- [x] 1.4 Run `go build ./... && go vet ./...` to confirm the shared callers (`cmd/ingest`, `cmd/backfill-role-fingerprint`, `internal/job`) still compile.

## 2. Rollout (ops — executed at Finish, low-traffic window)

- [ ] 2.1 After deploy, run `cmd/backfill-role-fingerprint` (tune `BACKFILL_CONCURRENCY`) to recompute every row's `role_fingerprint`; confirm `scanned`/`updated` counts are logged.
- [ ] 2.2 Run `make reindex` on its own flock (not stacked with the semantic or companies reindex) to collapse newly-clustered reposts and union their geography.
- [ ] 2.3 Verify on prod: the Towa "Senior Fullstack Engineer" cluster collapses its markup-only variants while Krakau (PLN) and the KV-clause Vienna posting stay distinct; spot-check a sample of the ~18k affected `(company, title)` groups.
