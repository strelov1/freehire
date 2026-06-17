# Tasks

## 1. Discoverer interface + run() branch

- [x] 1.1 Add the opt-in `discoverer` interface to
      `cmd/harvest-boards/prober.go`. In `main.go`, make `run()` accept the
      `harvest-boards <provider>` (2-arg) form: when the provider's prober
      implements `discoverer`, the candidate ids come from `discover(...)`;
      otherwise (or with 3 args) the seed path is unchanged; a 2-arg run for a
      non-discoverer prober is a usage error. Drive with a test asserting the
      branch selection (a fake discoverer prober supplies candidates without a
      seed; a non-discoverer with no seed errors).

## 2. gupyProber

- [x] 2.1 Add `gupyProber` with `probe(ctx, c, companyId)` →
      `(careerPageName, total)` via `?companyId=<id>&limit=1` (name falls back to
      id; zero/error → skip). Table-test the mapping with a fake `httpClient`.
- [x] 2.2 Add `gupyProber.discover(ctx, c)` paging the global feed
      (`?limit=&offset=`), collecting distinct `companyId`s in first-seen order,
      stopping on an empty page or `gupyMaxOffset`. Test: multi-page feed with
      repeated companies → distinct ids; empty-page termination; max-offset bound.
- [x] 2.3 Register `probers["gupy"] = gupyProber{}`; assert (test) the registry
      resolves `gupy` and that it implements `discoverer`.

## 3. Verification

- [x] 3.1 `go build ./... && go vet ./... && go test ./...` green; gofmt clean;
      `openspec validate gupy-board-discovery --strict` passes. Optional live
      smoke: `go run ./cmd/harvest-boards gupy` against a scratch copy enumerates
      and appends boards (review diff; do not commit the bulk append in this PR).
