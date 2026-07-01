## Why

The `sber` adapter builds every vacancy's apply URL from the vacancy's `requisitionId`
GUID (`https://rabota.sber.ru/search/<requisitionId>`), but that route returns HTTP 404
for **every** Sber posting — alive or not. Sber's public vacancy page is addressed only by
the numeric `internalId`. As a result every Sber apply link on freehire is broken, which
also makes live vacancies look permanently removed.

## What Changes

- The `sber` adapter builds the job `URL` from the posting's numeric `internalId`
  (`https://rabota.sber.ru/search/<internalId>`, which 302-redirects to the SEO-slug page)
  instead of the `requisitionId`.
- `sberVac` gains an `internalId` field (already present in the feed payload) to carry it.
- The dedup key is unchanged: `ExternalID` stays the `requisitionId`, so no re-keying or
  duplicate rows.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `source-ingest`: the `sber` inline-body scenario is refined to require the apply URL be
  built from the numeric `internalId`, not the `requisitionId`.

## Impact

- `internal/sources/sber.go` (URL construction + `sberVac` struct), `internal/sources/sber_test.go`.
- Existing Sber rows in prod keep their stale `url`; a re-ingest overwrites `url` via
  `UpsertJob` on the next Sber crawl. No migration needed.
