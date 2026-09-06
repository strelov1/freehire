## 1. Adapter core (`internal/ingest/sources`)

- [x] 1.1 Token mint: GET `https://<board>/jobs`, extract the `"token":"eyJ..."` JWT from the
      embedded `__NEXT_DATA__` blob; error if none is found (board unreachable or platform
      changed shape).
- [x] 1.2 GraphQL search + cursor pagination: POST `https://<board>/graphql` with the
      `searchJobs`/`searchJobPostings` query, Bearer-authorized, paging via `after`/`endCursor`
      and stopping on `hasNextPage: false` or an empty `nodes` page.
- [x] 1.3 Posting → `Job` mapping: external id from `key`, `Title`, sanitized `descriptionHTML`,
      location from `primaryPlace`/`places` (joined free text), `WorkMode`/`Remote` from
      `workplaceType` (`ON_SITE`/`REMOTE`/`HYBRID`), `EmploymentType` from `positionType.name`
      ("Full Time"/"Part Time" only), `PostedAt` from `postedOn`, `Company` from the board's
      `CompanyEntry`.
- [x] 1.4 Register `NewGr8People` under provider key `gr8people` in `registry.go`.

## 2. Public URL recognition (`internal/ingest/atsboard`)

- [x] 2.1 Add `{"gr8people", "gr8people", modeHost}` and `{"workgr8", "gr8people", modeHost}` to
      the `atsBoards` table.
- [x] 2.2 Add `TestRecognize` cases: a `*.gr8people.com` job URL and a `*.workgr8.com` job URL
      both resolve to `(gr8people, <host>)`; a bare apex host (no tenant subdomain) on either
      domain is not recognized.

## 3. Documentation

- [x] 3.1 Add a "gr8people traps" section to `internal/ingest/sources/AGENTS.md` (same style as
      the Dayforce/jobappnetwork entries): the two-domain-one-vendor confirmation, the token-mint
      mechanism and its per-tenant scoping, the GraphQL query shape, why no visibility filter is
      needed (unlike jobappnetwork), and the custom-fields non-goal.

## 4. Verification

- [x] 4.1 `gofmt -l` the changed/new Go files (must print nothing).
- [x] 4.2 `go build ./... && go vet ./...`.
- [x] 4.3 `go test ./internal/ingest/sources/... ./internal/ingest/atsboard/...` green, including
      new tests for tasks 1.1-1.3 and 2.2.
- [x] 4.4 `make gen-contracts` and commit the regenerated `web/src/lib/generated/contracts.ts` —
      the jobappnetwork PR's CI failure showed this is required whenever a new provider key is
      added.
