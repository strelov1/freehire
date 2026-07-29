## 1. Sweep grace window

- [x] 1.1 Add the `sweepGrace` marker interface to `internal/sources/source.go`, documented in the style of `selfClosing`/`fullCatalog`: an adapter declares a window wider than the sweep default when its crawl deliberately covers only a slice of its catalogue
- [x] 1.2 Add `sources.SweepGraceWindows(reg map[string]Source) map[string]time.Duration` to `internal/sources/registry.go`, mirroring `SelfClosingProviders`, with a test proving a declaring adapter appears and a non-declaring one does not
- [x] 1.3 Make `cmd/ingest` compute the sweep cutoff from the provider's declared window, falling back to the existing 48-hour default; test that a declaring provider's recently-drifted job survives and a default provider's behaviour is unchanged

## 2. Feed client

- [x] 2.1 Create `internal/sources/whatjobs.go` with `NewWhatJobs(c HTTPClient, publisherID string)` and `Provider() string` returning `whatjobs`; assert the `aggregator()` marker and the absence of `boardless()`/`fullCatalog()`
- [x] 2.2 Build the feed request from a board entry: `publisher`, `user_ip` placeholder, `keyword` from the board, `limit=50`, `page`; assert no `user_agent` parameter is sent and no value contains a slash
- [x] 2.3 Decode the response envelope (`data`, `total`, `current_page`, `last_page`) from a recorded fixture, including the `snippet`-as-full-description shape
- [x] 2.4 Surface a rejected publisher id (HTTP `410`, `Invalid Publisher` body) as a board-level error so the run counts it failed and closes nothing

## 3. Posting identity and normalization

- [x] 3.1 Extract the native id from the tracked URL's `pub_api__cpl__(\d+)__` segment; skip a posting whose URL lacks it rather than storing a guessed id
- [x] 3.2 Store the tracked URL unchanged as the job's posting URL, with a test pinning that the query string is preserved so publisher attribution survives
- [x] 3.3 Strip the trailing `#J-<digits>-Ljbffr` reseller signature from the description; test a signed and an unsigned description
- [x] 3.4 Discard `salary`, `job_type` and `logo`; test that the placeholder salary `0.000000 - 0.000000` yields no salary
- [x] 3.5 Leave the posted date unset — assert `age`/`age_days` never reach it, so freshness falls back to `created_at`
- [x] 3.6 Compose the location from the posting's city plus the account's country, and carry the US ZIP through; test that a bare city like `London` is not left to be read as the UK one

## 4. Pagination

- [x] 4.1 Page a keyword until an empty page, the feed's depth ceiling, or the 40-page budget; test that a short page (44 rows for a requested 50) does NOT end pagination
- [x] 4.2 Log once when the page budget is what stopped a crawl, so a bounded slice never reads as full coverage
- [x] 4.3 Declare the 14-day `sweepGrace` window on the adapter and cover it with a test

## 5. Wiring

- [x] 5.1 Register `whatjobs` in `sources.All` only when `WHATJOBS_PUBLISHER_ID` is non-empty, following the `usajobs`/`reed` pattern; test both the configured and unconfigured registry
- [x] 5.2 Create `sources/whatjobs.yml` with the seed keyword slices, each entry carrying a display-label `company` and the keyword as `board`, headed by a comment explaining the keyword-as-board shape and the US-only account
- [x] 5.3 Verify the board file against the registry the way `cmd/ingest` does, so a malformed entry fails fast

## 6. Verification

- [x] 6.1 Run `go build ./... && go vet ./... && go test ./...` green
- [x] 6.2 Run one live board end to end against the real feed with the publisher id set, and confirm the stored rows: description free of the reseller signature, no salary, unset posted date, URL intact, country present
- [x] 6.3 Document the provider in `internal/sources/AGENTS.md` — the keyword-as-board shape, the env-only publisher id, and the two feed traps (slash in `user_agent`, `limit=1` with a keyword)

## 7. Review findings

- [x] 7.1 Refuse a blank (whitespace-only) keyword before any request — config validation only rejects a strictly empty board, and the feed answers a blank keyword with its whole unfiltered inventory (32k postings for `"   "`, 560k for `""`)
- [x] 7.2 Send the keyword trimmed, so a padded board entry still crawls the intended slice
- [x] 7.3 Fail the board when a page returns postings but none carries a recognizable native id — a silent empty result would be invisible, since a provider with zero ingested jobs is skipped by the sweep and its rows would stay open forever
