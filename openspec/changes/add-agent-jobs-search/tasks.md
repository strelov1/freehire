## 1. Shared search core

- [ ] 1.1 Extract the public `SearchJobs` request handling (filter/sort/pagination
  window/semantic ratio → `a.search.Search`) into one internal helper returning the
  `search.SearchResult` (hits + total), and have `SearchJobs` call it.
- [ ] 1.2 Confirm the existing `internal/handler/search_test.go` suite stays green
  after the extraction (the public endpoint's behavior is unchanged).

## 2. Description hydration

- [ ] 2.1 Add `GetJobDescriptionsByIDs :many` (`SELECT id, description ... WHERE id
  = ANY($ids)`) to `internal/db/queries/jobs.sql`; regenerate sqlc.
- [ ] 2.2 Add a handler helper that, given the hits, batch-loads full descriptions
  by id and patches each hit best-effort (missing id keeps the index value, never
  drops the hit).

## 3. Format conversion

- [ ] 3.1 Add a pure `formatDescription(html string, format string) string` helper:
  `html` (identity), `text` (strip tags), `markdown` (HTML→Markdown preserving
  block structure); unrecognized → `html`.
- [ ] 3.2 Add the HTML→text/markdown conversion dependency (or std-lib strip for
  `text`); keep it behind the helper.
- [ ] 3.3 Unit-test the helper for all three formats plus the fallback.

## 4. Endpoint

- [ ] 4.1 Add `AgentSearchJobs` handler: run the shared search core, hydrate full
  descriptions, apply `description_format`, respond with the `{data, meta}` envelope
  (public_slug, no internal id).
- [ ] 4.2 Register `GET /api/v1/agent/jobs/search` as a public (no-auth) route.
- [ ] 4.3 Unit-test the endpoint with a fake searcher + fake description loader:
  full description replaces the preview; best-effort keeps a stale hit; each format
  transforms `description`; default is verbatim html.

## 5. Verification & docs

- [ ] 5.1 `go build ./... && go vet ./... && go test ./...` pass; gofmt clean.
- [ ] 5.2 Document `GET /api/v1/agent/jobs/search` and its `description_format`
  parameter in the `api-documentation` surface (`web/src/lib/docs/api-spec.ts`,
  regenerate `docs/API.md`).

## 6. Close the loop

- [ ] 6.1 Offer a `/blog` changelog entry (user-facing API addition) per the
  announce-shipped-work convention.
