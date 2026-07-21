## Why

A Go naming review of `internal/` + `cmd/` found the codebase almost entirely
clean, with four small identifier-naming deviations from the project's Go
conventions. Fixing them keeps the naming baseline consistent so the deviations
don't get copied as precedent.

## What Changes

- Prefix the `errMissing` sentinel string in `cmd/harvest-boards/prober.go` with
  the package name: `"not found"` → `"harvest: not found"` (sentinel errors
  identify their origin when wrapped).
- Rename boolean field `jobreality.Input.EvergreenText` → `HasEvergreenText`
  (boolean fields take an `is`/`has`/`can` prefix).
- Rename boolean field `jobreality.Evidence.FakeFreshness` → `IsFakeFreshness`.
  This value flows through the `jobview.Reality` wire struct; the JSON tag stays
  `fake_freshness`, so the wire contract is unchanged.
- Rename boolean field `inboxFilters.Unread` → `IsUnread` in
  `internal/handler/inbox.go`.

All four are mechanical renames covered by existing tests. No behavior change,
no API/wire-contract change.

## Capabilities

### New Capabilities
<!-- none — refactor only -->

### Modified Capabilities
<!-- none — no spec-level behavior changes; identifier renames only -->

## Impact

- Code only: `cmd/harvest-boards/prober.go`, `internal/jobreality/classify.go`,
  `internal/jobview/reality.go`, `internal/handler/inbox.go`, plus the tests
  that reference the renamed fields.
- No database, migration, API response, or JSON wire-tag changes.
