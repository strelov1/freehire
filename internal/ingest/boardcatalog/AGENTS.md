# Board catalog conventions

## Scope
The `boards` table: which company crawls on which ATS, under what board id, and whether
that board is proven. It replaced `sources/*.yml` (#2357) as what `cmd/ingest` reads, and
absorbed the recognized half of `internal/ingest/contribution`'s lifecycle.

## Always true
- **The catalog is the schedule and the crawl list.** `cmd/ingest <provider>` reads
  `status IN ('pending','active')` for that provider; `deploy/bin/gen-ingest-timers.sh`
  reads the distinct providers of the same set. A board is crawled because it has a live
  row, and a provider is scheduled because it has one — neither depends on a file.
- **Nothing writes a row directly.** Every insert goes through `Insert`, which validates,
  normalizes, and checks for a duplicate before it persists. A raw `INSERT` skips all
  three. The three writers are the site's contribution flow (`pending`), `cmd/add-board`
  (`active` — a curator has already verified it), and the harvest tools (`pending`).
- **`pending` boards ARE crawled.** Pending means unproven, not untested:
  `pipeline.Runner` flips `pending → active` the first time a board's crawl completes
  without a board-level error. A board that never activates is a board that has never
  succeeded, which is exactly the signal the status exists to carry.
- **Retirement is a status, never a delete.** `retired` leaves the row and its history in
  place, and does not occupy the identity — so re-adding a board that was retired by
  mistake is never blocked by its own retirement. Same for `rejected`: a corrected
  resubmission after a validation failure is never blocked by the earlier typo.

## Duplicate identity is TWO checks, and only one of them is the index
`boards_identity_key` is `UNIQUE (provider, lower(board), region) WHERE status IN
('pending','active')`. It folds CASE, and that is all SQL can do here.

It cannot fold the FORM of a board id, because the fold is Go: iCIMS writes one board as
both `vet` and `careers-vet.icims.com`; Dayforce's optional culture segment names one site
twice; Gusto resolves an old and a new employer slug to one uuid; UKG Ready serves one
tenant from several pod hosts. `sources.BoardDedupeKey` folds all four, and `Insert`
compares it against the provider's live boards before writing.

The cost of missing it is not a wasted crawl. The pipeline namespaces `external_id` with
the LITERAL board string, so two spellings store the same postings twice under one
`company_slug` — and the post-run unseen sweep is scoped by `company_slug`, not by board,
so a run that refreshes one spelling closes the other's still-live rows. A false-close, not
a duplicate.

## Structure
- `catalog.go` — `Status`, `InsertInput`, `Board`, and `Validate` (the registry check,
  shared with what `Config.Validate` runs).
- `repository.go` — the `Repository` port, `Insert` (validate → normalize → dedupe →
  persist), and `QueriesRepository` over sqlc.
- `loader.go` — `Board.CompanyEntry()` and `LoadForProvider`, the projection `cmd/ingest`
  feeds to `pipeline.Runner`.
- `placeholder.go` — the company name a URL-only crowdsourced submission gets until a
  curator renames it (`cmd/add-board --rename`).

## Gotchas
- **`Insert` never returns a validation error.** An invalid candidate is PERSISTED at
  `status='rejected'` with the reason, so the submitter is told why instead of the row
  silently not existing. Only `ErrDuplicateBoard` and genuine persistence failures come
  back as errors — check `b.Status == StatusRejected` if you care.
- **`Insert` costs one extra query.** The FORM-fold check lists the provider's live boards.
  That is fine on every current caller (a curator addition, one submission, a harvest that
  already listed them) and would not be on a hot path.
- **The company in a catalog row is not the company on the posting.** Many adapters take
  the employer name from the payload — on `jazzhr` only 2453 of 3940 companies match their
  row — so never key anything on `normalize.CompanySlug(board.Company)`. The board is the
  identity that joins exactly.
