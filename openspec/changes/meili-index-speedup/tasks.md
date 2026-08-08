## 1. Tune facet index settings

- [x] 1.1 Add a failing unit test in `internal/search/settings_test.go` asserting
      `facetSettings().ProximityPrecision == meilisearch.ProximityPrecisionByAttribute`
      and `facetSettings().PrefixSearch == meilisearch.PrefixSearchDisabled` (check the
      exact meilisearch-go SDK constant/type names for these two fields before writing
      the assertions — the SDK version is pinned in `go.mod`).
      Actual pinned SDK (v0.36.3) names: `ProximityPrecision` is typed
      `meilisearch.ProximityPrecisionType`, constant `meilisearch.ByAttribute`;
      `PrefixSearch` is a raw `*string` with no SDK constant (value `"disabled"`).
- [x] 1.2 Set `ProximityPrecision` and `PrefixSearch` on the `meilisearch.Settings`
      returned by `facetSettings()` in `internal/search/client.go` to make the test
      pass.
      **Reverted during code review**: `PrefixSearch` was dropped entirely.
      `HeaderSearch.svelte` and the `/jobs` list's `filters.ts` both debounce a
      query-as-you-type search through this index, relying on Meilisearch's default
      last-word prefix matching — disabling it would have broken mid-word live
      search. Only `ProximityPrecision: byAttribute` shipped; see design.md
      Decision 2. The test and its assertions were updated to match (now asserts
      `PrefixSearch` stays nil).
- [x] 1.3 Confirm `TestFacetAndSemanticShareKeywordSettings` (settings_test.go:81) and
      the rest of `internal/search`'s existing settings tests still pass — this
      change must not touch `semanticSettings()` (see design.md Non-Goals).
      Extended this test to also assert `ProximityPrecision` parity between
      `facetSettings()` and `semanticSettings()`, per code review.

## 2. Verify

- [x] 2.1 `go build ./... && go vet ./...`
- [x] 2.2 `go test ./...`
- [x] 2.3 `go vet -tags=integration ./...` (per repo AGENTS.md — cheap guard before
      push; `internal/search` carries integration-tagged tests, e.g.
      `search_integration_test.go`)
- [x] 2.4 If Docker is available, run the package's integration tests
      (`go test -tags=integration ./internal/search/...`) to catch any Meilisearch
      SDK/server incompatibility with the two new setting values against the pinned
      `getmeili/meilisearch:v1.49.0` (docker-compose.yml) — both settings are
      confirmed supported at that version per the design doc's research.
      Ran against the integration suite's pinned image (`getmeili/meilisearch:v1.13`,
      via testcontainers) — all tests passed, including the new settings assertion.
