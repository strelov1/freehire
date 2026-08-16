## 1. Classify the fault

- [x] 1.1 Write the failing table test for the classifier: a corrupted-row error,
      an unparseable model response, and a validation failure are the posting's
      fault; a gateway 502, a 500, a 401, a timeout, a write-back error and an
      unrecognised error are not. Include the two real production error strings
      verbatim — the classifier's whole job is to answer correctly for those.
- [x] 1.2 Implement the classifier in `internal/enrich`. It enumerates the
      failures the package itself raises; the default branch is "not the
      posting's fault", and a comment says why that direction is the safe one.

## 2. Bound the two classes differently

- [x] 2.1 Add `ENRICH_UPSTREAM_GRACE` (default 14 days) to
      `internal/config/enrich.go` with its test, alongside the existing
      `ENRICH_MAX_ATTEMPTS`.
- [x] 2.2 Write the failing test for the failure statement: an entry failing on a
      posting fault dead-letters at the attempt maximum; an entry failing on our
      fault does not, however many attempts it has, until it is older than the
      grace window. Integration-tagged — this is SQL behaviour.
- [x] 2.3 Extend `RecordEnrichmentFailure` to take the grace window and whether
      the posting is at fault, and decide `failed_at` from both. `created_at` is
      already on the table, so no migration. Run `make sqlc`. A pre-existing test
      caught a hazard introduced here: read as arithmetic, a zero grace window
      buries every entry on its first failure, so a non-positive window now means
      "never bury on age" and has its own test.
- [x] 2.4 Write the failing runner test: `process` classifies the cause and passes
      the right bound, and the corrupted-row path keeps its immediate
      dead-letter. No runner test for an invalid payload: `Sanitize` clears
      exactly the fields `Validate` checks, so that branch cannot fire through
      the real path and asserting on it would mean contriving an input
      production never produces.
- [x] 2.5 Wire the classifier through `fail`/`failN` in `internal/enrich/runner.go`
      and pass the grace window from `cmd/enrich`.

## 3. Verify against the real failure

- [x] 3.1 Write the integration test that reproduces the July outage in miniature:
      an entry fails with the production 502 string more times than
      `ENRICH_MAX_ATTEMPTS`, and is still claimable afterwards. This is the
      regression the whole change exists to prevent, so it gets its own test
      rather than being implied by 2.2.

## 4. Finish

- [x] 4.1 `gofmt -w` the touched files, then `go vet ./...`, `go test ./...`, and
      `go vet -tags=integration ./...`.
- [x] 4.2 Run the tagged suite for the changed packages
      (`go test -tags=integration ./internal/db/ ./internal/enrich/`).
- [x] 4.3 Record the retry policy in `internal/enrich/AGENTS.md`: which failures
      are the posting's fault, why the default is the other way, and why our
      faults are bounded by time rather than attempts.
- [x] 4.4 Put the one-off requeue statement in the PR description with the
      measurement that justifies its `WHERE`, and state that it runs only after
      the policy is deployed.
