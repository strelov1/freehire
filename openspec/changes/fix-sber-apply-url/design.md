## Context

`internal/sources/sber.go` maps each feed publication to a `Job`. It sets both the dedup
`ExternalID` and the apply `URL` from `sberVac.RequisitionID`:

```go
sberVacURL = "https://rabota.sber.ru/search/%s"   // formatted with v.RequisitionID
```

A spike against live Sber data showed the `requisitionId` GUID route 404s for every
vacancy, while `https://rabota.sber.ru/search/<internalId>` returns 200 and 302-redirects
to the SEO-slug page. The feed record already carries the numeric `internalId` (confirmed
in the raw payload); the adapter's `sberVac` struct simply does not parse it today.

## Goals / Non-Goals

**Goals:**
- Emit a working apply URL for every Sber vacancy, built from `internalId`.
- Preserve the dedup key (`ExternalID` = `requisitionId`) so no rows re-key or duplicate.

**Non-Goals:**
- No liveness/closure changes — a corrected URL merely unblocks a future liveness probe;
  that is out of scope here.
- No SEO-slug reconstruction. Bare `search/<internalId>` 302s to the slug page, which is
  sufficient; storing the slug form is not worth reimplementing Sber's slugging.

## Decisions

- **Add `InternalID int64` to `sberVac`** and format the URL from it. The feed field is
  `internalId` (a JSON number), so parse it as an integer, not a string.
  - Alternative: reuse `publicationId` (also a GUID) — rejected, it 404s like
    `requisitionId`. Only `internalId` resolves.
- **Keep `ExternalID` on `requisitionId`.** It is the existing dedup identity for all Sber
  rows; switching it would orphan every current row and re-ingest duplicates.

## Risks / Trade-offs

- [Existing prod rows keep the stale 404 URL until re-crawled] → `UpsertJob` overwrites
  `url` on the next Sber crawl; no backfill needed, the fix self-heals within one cycle.
- [Sber could change its URL scheme again] → covered by the new spec scenario and a unit
  test asserting the `internalId` URL shape, so a regression is caught.

## Migration Plan

None. Ship the adapter change; the next scheduled Sber crawl overwrites `url` for every
posting via the normal upsert path. Rollback is reverting the adapter.

## Open Questions

None.
