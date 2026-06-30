## 1. Test fixtures

- [ ] 1.1 Capture a trimmed real `/api/offers` list response (a few offers spanning single-mode,
  mixed-mode, relocation-only-plus-one-mode, and a one-day-offer card) and a `/api/offers/{id}`
  detail response into `internal/sources/testdata/` for the adapter test.

## 2. Adapter core

- [ ] 2.1 Define the response/offer structs and the `getmatch` adapter type with `Provider()`,
  `boardless()`, `aggregator()`, and `NewGetmatch(c JSONGetter)` (mirroring `tecla`).
- [ ] 2.2 Implement `Fetch` pagination over `/api/offers?offset=&limit=100` stopping at
  `meta.total` with a `maxPages` bound, and map each offer to `Job` (`ExternalID`=`id`,
  `URL`=absolute, `Title`=`position`, `Company`=`company.name`, `Location`=distinct labels,
  `PostedAt`=`published_at`); first-page failure errors, later-page failure ends enumeration.
- [ ] 2.3 Fetch the per-offer detail `/api/offers/{id}`, use its sanitized HTML `description`,
  and fall back to the list `offer_description` when the detail description is empty or the
  detail request fails (never drop the offer).
- [ ] 2.4 Derive the structured `WorkMode` from `location_items[].format`
  (`remote`/`hybrid`/`office`→`remote`/`hybrid`/`onsite`, ignoring `relocation_*`), emitting it
  only on a single distinct work mode; set `Remote` iff the work mode is `remote`.

## 3. Registration and config

- [ ] 3.1 Register `getmatch` in `sources.All` (one `NewGetmatch(c)` line).
- [ ] 3.2 Add `sources/getmatch.yml` with one boardless placeholder entry.

## 4. Verification

- [ ] 4.1 `go build ./... && go vet ./... && go test ./internal/sources/` all green, and a live
  smoke run of the adapter against the real API yields normalized jobs with full descriptions.
