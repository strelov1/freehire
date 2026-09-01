## 1. The query

- [ ] 1.1 Add `ProviderIngestFreshness :many` to
  `internal/platform/db/queries/metrics.sql` — `provider, max(last_success_at)` grouped by
  provider — with a comment stating why it reads `board_health` and not `jobs` (41s vs
  54ms, measured on prod 2026-09-01).
- [ ] 1.2 Run `make sqlc` and commit the regenerated `internal/platform/db`.

## 2. Collection

- [ ] 2.1 Write the failing test in `cmd/queue-metrics/collect_test.go`: the fake returns
  two providers, one with a timestamp and one with NULL, and `collect` carries both into
  the snapshot with the NULL distinguishable from a zero instant.
- [ ] 2.2 Add the method to the `metricsQueries` interface, the field to `snapshot`, and
  the call to `collect` — failing the whole pass on error, as every other query there does.

## 3. Rendering

- [ ] 3.1 Write the failing test in `cmd/queue-metrics/render_test.go` pinning the exact
  exposition text for the new family, including the `provider` label — the existing tests
  pin text because the metric names are a cross-repo contract with `freehire-ops`.
- [ ] 3.2 Write the failing test that a provider with no successful crawl emits **no
  sample**, not a zero.
- [ ] 3.3 Write the failing test that providers are emitted in a stable order, so the
  exposition does not churn between scrapes.
- [ ] 3.4 Implement the family in `render`.

## 4. Verify

- [ ] 4.1 `gofmt -l .` prints nothing; `go vet ./...`; `go test ./...`;
  `go vet -tags=integration ./...`.
- [ ] 4.2 `make sqlc` leaves no diff (the CI job and pre-commit hook both check this).
- [ ] 4.3 Run `cmd/queue-metrics` against prod read-only with `PROM_TEXTFILE_DIR` set to a
  temp directory and confirm the emitted family lists the providers and that the known-dead
  ones (`recruitingsolutions`, `careerspage`, `alfabank`) carry timestamps weeks old.
- [ ] 4.4 State in the PR that the alert rule is a `freehire-ops` change and is not
  included, so the gauge is not mistaken for the alert.
