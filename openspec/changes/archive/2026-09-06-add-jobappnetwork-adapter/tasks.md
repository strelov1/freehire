## 1. Adapter core (`internal/ingest/sources`)

- [x] 1.1 Board-id validation: a board must be a positive integer client id; empty, non-numeric,
      zero or negative is rejected with an error before any request is issued.
- [x] 1.2 Listing request + pagination: build the `proxy-es/search-en-us/posting/_search` POST
      body filtered on `clientId` and `internalOrExternal: externalOnly`, page with `from`/`size`,
      and stop once the accumulated hits reach the response's `hits.total`.
- [x] 1.3 Posting → `Job` mapping: external id from `jobId`, `Title`, sanitized `description` as
      the body, structured location from `address` (city/state/country), `Countries` from the
      country code, `PostedAt` from `createdDate`, `Company` from the board's `CompanyEntry` (not
      from `clientName`).
- [x] 1.4 Register `NewJobAppNetwork` under provider key `jobappnetwork` in `registry.go`, wired
      over the shared keyless JSON client (`JSONPoster`).

## 2. Public URL recognition (`internal/ingest/atsboard`)

- [x] 2.1 Add `{"apply.jobappnetwork.com", "jobappnetwork", "clients"}` to the `apiBoards` table.
- [x] 2.2 Add `atsdetect`/`FromURL` test cases: a well-formed
      `apply.jobappnetwork.com/clients/<id>/posting/<id>/` link resolves to
      `(jobappnetwork, "<id>")`; a link on the host with no `/clients/<id>/…` path is not
      recognized.

## 3. Documentation

- [x] 3.1 Add a "jobappnetwork traps" section to `internal/ingest/sources/AGENTS.md` (same style
      as the existing Dayforce/Workstream/EDJOIN entries): the real API host and endpoint, the
      request/response shape, the `internalOrExternal` filter and why it is mandatory, the
      single-employer-per-clientId board model, and the multi-tenant-exposure note from
      design.md's Context (informational — not something this adapter needs to defend against
      beyond always sending the `clientId` filter).

## 4. Verification

- [x] 4.1 `gofmt -l` the changed/new Go files (must print nothing).
- [x] 4.2 `go build ./... && go vet ./...`.
- [x] 4.3 `go test ./internal/ingest/sources/... ./internal/ingest/atsboard/...` green, including
      new tests for tasks 1.1-1.3 and 2.2.
