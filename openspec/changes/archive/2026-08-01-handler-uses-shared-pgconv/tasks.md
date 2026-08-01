## 1. Delete the twins

- [x] 1.1 Confirm each local helper is body-identical to its pgconv counterpart before deleting
      it — that identity is what makes this a deletion rather than a behaviour change.
- [x] 1.2 Delete `tsFromPtr`; call `pgconv.Timestamptz` at its one call site.
- [x] 1.3 Delete `int4Ptr`; call `pgconv.IntPtr` at its one call site, and drop the now-unused
      `pgtype` import from that file.
- [x] 1.4 Found while implementing: `api_keys.go` inlines the same conversion as a `var` plus an
      `if`. Collapse it to one `pgconv.Timestamptz` call.

## 2. Verify and close

- [x] 2.1 `go build ./... && go vet ./... && go test ./...` green.
- [x] 2.2 Survey every remaining `pgtype` literal in the package and record which are genuinely
      outside pgconv's scope, so the next reader does not mistake them for missed twins.
- [x] 2.3 Mark S12 ✅ in `docs/reviews/2026-08-01-architecture-review.md`, noting that its
      "handler never imports pgconv" evidence went stale when #1409 added the first import.
