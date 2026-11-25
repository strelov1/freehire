## 1. `q_fields` validation in `internal/search/search`

- [x] 1.1 Write failing tests in `query_params_test.go` for a new helper that parses
      a comma-separated `q_fields` value against `{title, company, description,
      location}`: valid names return the canonical-order attribute list; any
      unrecognized name (alone or mixed with valid ones) returns no restriction and
      reports `q_fields` via the existing `UnknownParam`/`SortAndCap` shape.
- [x] 1.2 Implement the helper in `query_params.go` (e.g. `QFieldsFromValues`),
      reusing `SortAndCap` for the report so it composes with the rest of
      `UnknownParams`'s output at the handler level.
- [x] 1.3 `go test ./internal/search/search/...` passes.

## 2. Query-time field scoping in `internal/search/search/client.go`

- [x] 2.1 Write failing tests exercising `buildSearchRequest` (or the nearest
      existing test covering it) with an `AttributesToSearchOn` field set: absent
      `q_fields` leaves it unset (today's behavior), present `q_fields` sets it to
      the canonical-order attribute list.
- [x] 2.2 Extend `SearchParams` with the resolved field list and set
      `AttributesToSearchOn` on the built request when non-empty.
- [x] 2.3 `go test ./internal/search/search/...` passes.

## 3. Wire `q_fields` through both search endpoints

- [x] 3.1 Write failing tests in `internal/api/handler` (`search_test.go`,
      `agent_search_test.go`) for `SearchJobs` and `AgentSearchJobs`, using the
      existing `fakeSearcher` — no `//go:build integration` tag needed here since
      `searcher` is a plain interface: a `q_fields=title` request narrows
      `SearchParams.QFields`; an unrecognized `q_fields` value is reported in
      `meta.ignored_params` and does not restrict.
- [x] 3.2 Read `q_fields` from the query string in the shared `runJobSearch`
      core, resolve it via `search.QFieldsFromValues`, and merge its ignored-param
      report into each handler's `meta.ignored_params` computation
      (`search.SortAndCap(append(qFieldsIgnored, ignoredParams(c, own)...))`).
- [x] 3.3 Add `q_fields` to a new `jobSearchParams` (searchParams + q_fields, used
      by `SearchJobs` and folded into `agentSearchParams`) rather than the shared
      `searchParams` the swipe deck also uses — the deck doesn't read `q_fields`
      and must keep reporting it as unknown.
- [x] 3.4 `go build ./... && go vet ./...` passes; `go test
      ./internal/api/handler/...` passes.

## 4. Documentation

- [x] 4.1 Update `web/static/openapi.yaml` for both `/jobs/search` and
      `/agent/jobs/search`: document `q`'s OR-of-tokens (unquoted) /
      order-independent-AND-of-tokens (quoted, not a phrase match) semantics, that
      `q` spans `title`/`company`/`description`/`location`, and the new `q_fields`
      parameter with its whole-value-drop-on-unrecognized-name behavior. Also
      updated `web/src/lib/docs/api-spec.ts` — the parallel source `docs/API.md`
      is generated from (discovered via the `docs/API.md`-freshness CI step) —
      and regenerated `docs/API.md` with `pnpm run gen:api-docs`.
- [x] 4.2 Validated with `npx @redocly/cli@2.49.0 lint web/static/openapi.yaml
      --extends=minimal` (the `artifacts` CI job's exact check): passes.

## 5. Verification

- [x] 5.1 `gofmt -l .` reports nothing for changed files.
- [x] 5.2 `go build ./... && go vet ./...` passes.
- [x] 5.3 `go test ./...` passes (whole module, no FAIL).
- [x] 5.4 Verified against a **real** Meilisearch (not a fake) rather than a manual
      curl session, so the check leaves a permanent regression test: added
      `TestSearchQFieldsRestrictsMatchingOnRealEngine` to
      `search_integration_test.go` (testcontainers, `//go:build integration`),
      reproducing issue #2671's own motivating example — "Engineer – Asset
      Manager" at "STS Systems Defense" — and confirming `q_fields=title`
      excludes the company-name-only match while an unscoped `q=systems` still
      surfaces it. `go test -tags=integration ./internal/search/search/...`
      passes (31 tests); `go vet -tags=integration ./...` passes.
