## 1. Make the rule a test

- [x] 1.1 RED: a test that scans the handler package for any file reading the offset query param
      outside the shared helper, with its population derived from behaviour (files that paginate
      at all) so a new endpoint is enrolled by existing. It must fail on `copies.go` today.
- [x] 1.2 Guard against a vacuous pass: assert the scan actually found the paginating files, so
      a broken scan cannot read as a clean suite.

## 2. One parse, one clamp

- [x] 2.1 GREEN: generalise `pageParamsMax(c, ceiling)` to `pageParamsBounded(c, fallback,
      ceiling)` so a caller with its own default can use it, and move the overflow rationale onto
      it — the helper is now the only place the param is read.
- [x] 2.2 `JobCopies` calls it with `defaultCopiesLimit` / `maxCopiesLimit`; its two hand-rolled
      lines become one. Limit behaviour is unchanged (50 default, 200 cap).
- [x] 2.3 Update `inbox.go` and `me_tracking.go` to the renamed helper — behaviour unchanged,
      both already passed the shared default implicitly.

## 3. Verify and close

- [x] 3.1 `go build ./... && go vet ./... && go test ./...` green.
- [x] 3.2 Confirm the defect was live before the fix, not theoretical: `offset=3000000000`
      answered 500 on production against a real job slug, `offset=0` answered 200.
- [ ] 3.3 After deploy, re-run the same two requests and confirm the 500 became a 200.
- [x] 3.4 Mark S11 ✅ in `docs/reviews/2026-08-01-architecture-review.md` — shortlist row, the
      `S11` heading, the Progress table — noting anything the finding got wrong.
