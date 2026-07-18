## Why

The `arbeitsagentur` adapter never sets a work mode, so its German postings show 0% work-mode
coverage and none surface under the remote filter until (if ever) enrichment fills it. The detail
page's `ng-state` JSON — already fetched for the description — carries a per-job
`jobdetail.homeofficemoeglich` boolean, so the remote signal is available for free.

## What Changes

- Read `jobdetail.homeofficemoeglich` from the same detail `ng-state` blob the adapter already
  parses for the description, and set `Remote` + `WorkMode` from it via the existing
  `workModeFromRemote` helper (`true` → remote, `false` → unset) — the same mapping `apple` uses
  for its home-office flag.
- No extra request: the flag rides the detail fetch that already happens per kept posting.

## Capabilities

### New Capabilities
<!-- None. -->

### Modified Capabilities
- `arbeitsagentur-source`: the job-mapping requirement now also derives the job's remote flag and
  work mode from the detail page's `homeofficemoeglich`.

## Impact

- **Touched code:** `internal/sources/arbeitsagentur.go` (+ `_test.go`) — the detail struct gains
  one field and `toJob` sets remote/work mode.
- **No migrations, no API changes, no new dependencies, no new requests.**
