## Why

The `tbank` source adapter ingests **zero IT vacancies**. Its `getVacancies` list
request sends empty filters, and T-Bank's `publisher` source silently defaults to a
single category — `tcareer_work_with_clients` (mass-hire client/collection roles) —
so the entire IT (`tcareer_it`) and back-office (`tcareer_back_office`) catalogues
are never crawled. For a tech job aggregator this is the opposite of what we want:
prod holds ~1277 field-sales/collections roles and not one engineering vacancy.

## What Changes

- The `tbank` adapter's list request crawls **all three top-level categories**
  (`tcareer_it`, `tcareer_back_office`, `tcareer_work_with_clients`) by passing them
  as a curated constant slice in `filters.category`, instead of the empty filter
  that defaulted to client roles only. A single offset-paginated loop then returns
  the full catalogue (verified live: 264 + 457 + 970 = 1691 in one filtered query).
- The false code comment claiming the `publisher` source "covers all roles" is
  corrected to document the category-filter requirement.
- No prod DB surgery: the existing non-IT rows stay (their category is still
  crawled), so nothing is force-closed by this change.

## Capabilities

### New Capabilities
- `tbank-source`: the T-Bank careers-API adapter — a boardless single-company
  source that crawls every top-level vacancy category via the `getVacancies` list
  filter and fetches each posting's description from the detail endpoint.

### Modified Capabilities
<!-- none: no existing spec's requirements change -->

## Impact

- `internal/sources/tbank.go` — list request gains the category filter; the
  `tbankSource`/comment provenance is corrected.
- `internal/sources/tbank_test.go` — covers the multi-category list request.
- Runtime: the next `cmd/ingest sources/custom.yml` run begins ingesting IT and
  back-office vacancies (~721 additional postings beyond the current client roles).
- No schema, API, or config change.
