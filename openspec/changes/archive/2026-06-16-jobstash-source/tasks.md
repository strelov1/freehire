# Tasks

## 1. Marker split (boardless vs aggregator)

- [x] 1.1 Add the opt-in `aggregator` marker interface to
      `internal/sources/source.go` and change `FilterableProviders` to exclude a
      provider only when it is `boardless && !aggregator` (single-company
      boardless stays excluded; aggregators stay listed). Cover with a unit test
      asserting an existing single-company boardless provider (e.g. `ozon`) is
      absent and the board-based providers are present.

## 2. JobStash adapter

- [x] 2.1 Add `internal/sources/jobstash.go`: `NewJobStash(c)` /
      `Provider() == "jobstash"`, implementing `boardless()` and `aggregator()`.
      `Fetch` paginates `/jobs/list?page=&limit=200` by `total`, decodes the
      inline list, and maps each posting to `sources.Job` per the design table
      (company ← `organization.name`, url passthrough, `shortUUID` external id,
      `locationType` → `workplaceTypeMode`, epoch-ms `timestamp` → `PostedAt`,
      composed sanitized-HTML description). Drive with a table-driven test over a
      captured JSON fixture in `testdata/`, served by an `httptest` server,
      covering: field mapping, pagination across pages to `total`, public-vs-
      protected `url`, work-mode from `locationType`, and `organization.name` →
      `company`.
- [x] 2.2 Register `NewJobStash(c)` in `sources.All` and assert (test) the
      registry resolves provider `jobstash` and that `FilterableProviders`
      includes `jobstash`.

## 3. Configuration + contract

- [x] 3.1 Add `sources/jobstash.yml` with one boardless entry (no `board`);
      verify it loads and validates against the registry (config validation test
      or `go run ./cmd/ingest sources/jobstash.yml` dry path — at minimum
      `parseSourcesFile` accepts the empty board for `jobstash`).
- [x] 3.2 Regenerate the web source-facet contract (`make gen-contracts`) so
      `jobstash` appears in the generated source list; commit the regenerated
      output.

## 4. Verification

- [x] 4.1 `go build ./... && go vet ./... && go test ./...` green; gofmt clean;
      openspec validate passes. Live smoke against the real JobStash API (gated
      test, run once then removed) fetched 3058 jobs with url=3058/3058 (null-url
      fallback verified), desc=3056, work-mode remote=1399/hybrid=639/onsite=701,
      and no empty company — confirming the decode shape matches production.
